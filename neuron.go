package neurgo

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/couchbaselabs/logg"
	"github.com/proxypoke/vector"
	"log"
	"sync"
	"time"
)

type Neuron struct {
	NodeId             *NodeId
	Bias               float64
	Inbound            []*InboundConnection
	Outbound           []*OutboundConnection
	Closing            chan chan bool
	DataChan           chan *DataMessage
	ActivationFunction *EncodableActivation
	wg                 *sync.WaitGroup
	Cortex             *Cortex
	weightedInputs     []*weightedInput
}

func (neuron *Neuron) Init() {
	if neuron.Closing == nil {
		neuron.Closing = make(chan chan bool)
	}

	if neuron.DataChan == nil {
		neuron.DataChan = make(chan *DataMessage)
	}

	if neuron.wg == nil {
		neuron.wg = &sync.WaitGroup{}
		neuron.wg.Add(1)
	}
}

func (neuron *Neuron) Run() {

	defer neuron.wg.Done()

	closed := false

	neuron.checkRunnable()
	neuron.createEmptyWeightedInputs()

	closed = neuron.primeAllRecurrentOutbound()
	if closed {
		neuron.closeChannels()
		return
	}

	for {
		select {
		case responseChan := <-neuron.Closing:
			closed = true
			responseChan <- true
			break
		case dataMessage := <-neuron.DataChan:
			neuron.receiveDataMessage(dataMessage)
			neuron.logPostReceivedDataMessage(dataMessage)
			if neuron.receiveBarrierSatisfied() {
				closed = neuron.feedForward()
			}
		}

		if closed {
			neuron.closeChannels()
			break
		}

	}

}

func (neuron *Neuron) Shutdown() {

	closingResponse := make(chan bool)
	neuron.Closing <- closingResponse
	response := <-closingResponse
	if response != true {
		log.Panicf("Got unexpected response on closing channel")
	}

	neuron.shutdownOutboundConnections()

	neuron.wg.Wait()
	neuron.wg = nil
}

func (neuron *Neuron) Copy() *Neuron {

	// serialize to json
	jsonBytes, err := json.Marshal(neuron)
	if err != nil {
		log.Fatal(err)
	}

	// new neuron
	neuronCopy := &Neuron{}

	// deserialize json into new neuron
	err = json.Unmarshal(jsonBytes, neuronCopy)
	if err != nil {
		log.Fatal(err)
	}

	return neuronCopy

}

func (neuron *Neuron) ConnectOutbound(connectable OutboundConnectable) *OutboundConnection {
	return ConnectOutbound(neuron, connectable)
}

func (neuron *Neuron) ConnectInboundWeighted(connectable InboundConnectable, weights []float64) *InboundConnection {
	return ConnectInboundWeighted(neuron, connectable, weights)
}

// Find the subset of outbound connections which are "recurrent" - meaning
// that the connection is to this neuron itself, or to a neuron in a previous
// (eg, to the left) layer.
func (neuron *Neuron) RecurrentOutboundConnections() []*OutboundConnection {
	result := make([]*OutboundConnection, 0)
	for _, outboundConnection := range neuron.Outbound {
		if neuron.IsConnectionRecurrent(outboundConnection) {
			result = append(result, outboundConnection)
		}
	}
	return result
}

func (neuron *Neuron) RecurrentInboundConnections() []*InboundConnection {
	result := make([]*InboundConnection, 0)
	for _, inboundConnection := range neuron.Inbound {
		if neuron.IsInboundConnectionRecurrent(inboundConnection) {
			result = append(result, inboundConnection)
		}
	}
	return result
}

// a connection is considered recurrent if it has a connection
// to itself or to a node in a previous layer.  Previous meaning
// if you look at a feedForward from left to right, with the input
// layer being on the far left, and output layer on the far right,
// then any layer to the left is considered previous.
func (neuron *Neuron) IsConnectionRecurrent(connection *OutboundConnection) bool {
	if connection.NodeId.LayerIndex <= neuron.NodeId.LayerIndex {
		return true
	}
	return false
}

// same as isConnectionRecurrent, but for inbound connections
// TODO: use interfaces to eliminate code duplication
func (neuron *Neuron) IsInboundConnectionRecurrent(connection *InboundConnection) bool {
	if neuron.NodeId.LayerIndex <= connection.NodeId.LayerIndex {
		return true
	}
	return false
}

func (neuron *Neuron) InboundUUIDMap() UUIDToInboundConnection {
	inboundUUIDMap := make(UUIDToInboundConnection)
	for _, connection := range neuron.Inbound {
		inboundUUIDMap[connection.NodeId.UUID] = connection
	}
	return inboundUUIDMap
}

func (neuron *Neuron) String() string {
	return JsonString(neuron)
}

func (neuron *Neuron) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		struct {
			NodeId             *NodeId
			Bias               float64
			Inbound            []*InboundConnection
			Outbound           []*OutboundConnection
			ActivationFunction *EncodableActivation
		}{
			NodeId:             neuron.NodeId,
			Bias:               neuron.Bias,
			Inbound:            neuron.Inbound,
			Outbound:           neuron.Outbound,
			ActivationFunction: neuron.ActivationFunction,
		})
}

func (neuron *Neuron) feedForward() (closed bool) {

	scalarOutput := neuron.computeScalarOutput(neuron.weightedInputs)

	neuron.weightedInputs = createEmptyWeightedInputs(neuron.Inbound)

	dataMessage := &DataMessage{
		SenderId: neuron.NodeId,
		Inputs:   []float64{scalarOutput},
	}

	closed = neuron.scatterOutput(dataMessage)
	return
}

func (neuron *Neuron) scatterOutput(dataMessage *DataMessage) (closed bool) {

	closed = false

	for _, outboundConnection := range neuron.Outbound {

		if outboundConnection.NodeId.UUID == neuron.NodeId.UUID {
			// if we are sending to ourselves, short-circuit
			// channel and just call function directly.

			neuron.receiveRecurrentDataMessage(dataMessage)
			if neuron.receiveBarrierSatisfied() {
				closed = neuron.feedForward()
			}

		} else {
			logPreSend(neuron.NodeId,
				outboundConnection.NodeId, dataMessage)

			select {
			case responseChan := <-neuron.Closing:
				closed = true
				responseChan <- true
				break
			case outboundConnection.DataChan <- dataMessage:
				logWeights(neuron)
				logPostSend(neuron.NodeId,
					outboundConnection.NodeId, dataMessage)
			}

		}

	}
	return

}

func (neuron *Neuron) outbound() []*OutboundConnection {
	return neuron.Outbound
}

func (neuron *Neuron) setOutbound(newOutbound []*OutboundConnection) {
	neuron.Outbound = newOutbound
}

func (neuron *Neuron) inbound() []*InboundConnection {
	return neuron.Inbound
}

func (neuron *Neuron) setInbound(newInbound []*InboundConnection) {
	neuron.Inbound = newInbound
}

func (neuron *Neuron) primeRecurrentOutbound(cxn *OutboundConnection) (closed bool) {

	inputs := []float64{0}
	dataMessage := &DataMessage{
		SenderId: neuron.NodeId,
		Inputs:   inputs,
	}

	if cxn.NodeId.UUID == neuron.NodeId.UUID {
		// we are sending to ourselves, so short-circuit the
		// channel based messaging so we can use unbuffered channels
		neuron.receiveRecurrentDataMessage(dataMessage)
		if neuron.receiveBarrierSatisfied() {
			msg := "Receive Barrier not expected to be satisfied yet"
			logg.LogPanic(msg)
		}

	} else {

		logPreSend(neuron.NodeId, cxn.NodeId, dataMessage)

		if cxn.DataChan == nil {
			log.Panicf("DataChan is nil for connection: %v", cxn)
		}

		select {
		case cxn.DataChan <- dataMessage:
		case <-time.After(time.Second):
			log.Panicf("Timeout sending to %v", cxn)
		case responseChan := <-neuron.Closing:
			closed = true
			responseChan <- true
		}
		logWeights(neuron)
		logPostSend(neuron.NodeId, cxn.NodeId, dataMessage)
	}

	return

}

// In order to prevent deadlock, any neurons we have recurrent outbound
// connections to must be "primed" by sending an empty signal.  A recurrent
// outbound connection simply means that it's a connection to ourself or
// to a neuron in a previous (eg, to the left) layer.  If we didn't do this,
// that previous neuron would be waiting forever for a signal that will
// never come, because this neuron wouldn't fire until it got a signal.
func (neuron *Neuron) primeAllRecurrentOutbound() (closed bool) {
	closed = false
	recurrentConnections := neuron.RecurrentOutboundConnections()
	for _, recurrentConnection := range recurrentConnections {
		closed := neuron.primeRecurrentOutbound(recurrentConnection)
		if closed {
			break
		}
	}
	return
}

func (neuron *Neuron) checkRunnable() {

	if neuron.NodeId == nil {
		msg := fmt.Sprintf("not expecting neuron.NodeId to be nil")
		panic(msg)
	}

	if neuron.Inbound == nil {
		msg := fmt.Sprintf("not expecting neuron.Inbound to be nil.  neuron: %v", neuron)
		panic(msg)
	}

	if neuron.Closing == nil {
		msg := fmt.Sprintf("not expecting neuron.Closing to be nil")
		panic(msg)
	}

	if neuron.DataChan == nil {
		msg := fmt.Sprintf("not expecting neuron.DataChan to be nil")
		panic(msg)
	}

	if neuron.ActivationFunction == nil {
		msg := fmt.Sprintf("not expecting neuron.ActivationFunction to be nil")
		panic(msg)
	}

	if err := neuron.validateOutbound(); err != nil {
		msg := fmt.Sprintf("invalid outbound connection(s): %v", err.Error())
		panic(msg)
	}

}

func (neuron *Neuron) validateOutbound() error {
	for _, connection := range neuron.Outbound {
		if connection.DataChan == nil {
			msg := fmt.Sprintf("%v has empty DataChan", connection)
			return errors.New(msg)
		}
	}
	return nil
}

func (neuron *Neuron) computeScalarOutput(weightedInputs []*weightedInput) float64 {
	output := neuron.weightedInputDotProductSum(weightedInputs)
	logmsg := fmt.Sprintf("%v raw output: %v", neuron.NodeId.UUID, output)
	logg.LogTo("NODE_STATE", logmsg)
	output += neuron.Bias
	logmsg = fmt.Sprintf("%v raw output + bias: %v", neuron.NodeId.UUID, output)
	logg.LogTo("NODE_STATE", logmsg)
	output = neuron.ActivationFunction.ActivationFunction(output)
	logmsg = fmt.Sprintf("%v after activation: %v", neuron.NodeId.UUID, output)
	logg.LogTo("NODE_STATE", logmsg)
	return output
}

// for each weighted input vector, calculate the (inputs * weights) dot product
// and sum all of these dot products together to produce a sum
func (neuron *Neuron) weightedInputDotProductSum(weightedInputs []*weightedInput) float64 {

	var dotProductSummation float64
	dotProductSummation = 0

	for _, weightedInput := range weightedInputs {
		inputs := weightedInput.inputs
		weights := weightedInput.weights
		inputVector := vector.NewFrom(inputs)
		weightVector := vector.NewFrom(weights)
		dotProduct, error := vector.DotProduct(inputVector, weightVector)
		if error != nil {
			t := "%T error performing dot product between %v and %v"
			message := fmt.Sprintf(t, neuron, inputVector, weightVector)
			panic(message)
		}
		dotProductSummation += dotProduct
	}

	return dotProductSummation

}

func (neuron *Neuron) dataChan() chan *DataMessage {
	return neuron.DataChan
}

func (neuron *Neuron) nodeId() *NodeId {
	return neuron.NodeId
}

func (neuron *Neuron) initOutboundConnections(nodeIdToDataMsg nodeIdToDataMsgMap) {
	for _, outboundConnection := range neuron.Outbound {
		if outboundConnection.DataChan == nil {
			dataChan := nodeIdToDataMsg[outboundConnection.NodeId.UUID]
			if dataChan != nil {
				outboundConnection.DataChan = dataChan
			}
		}
	}
}

func (neuron *Neuron) shutdownOutboundConnections() {
	for _, outboundConnection := range neuron.Outbound {
		outboundConnection.DataChan = nil
	}
}

func (neuron *Neuron) receiveBarrierSatisfied() bool {
	return receiveBarrierSatisfied(neuron.weightedInputs)
}

func (neuron *Neuron) receiveDataMessage(dataMessage *DataMessage) {

	recordInput(neuron.weightedInputs, dataMessage)
}

func (neuron *Neuron) receiveRecurrentDataMessage(dataMessage *DataMessage) {
	logRecurrentSend(neuron.NodeId, dataMessage)
	neuron.receiveDataMessage(dataMessage)
	logRecurrentRecv(neuron.NodeId, dataMessage)
}

func (neuron *Neuron) logPostReceivedDataMessage(dataMessage *DataMessage) {
	neuron.logReceivedDataMessage(dataMessage, "NODE_POST_RECV")
}

func (neuron *Neuron) logReceivedDataMessage(dataMessage *DataMessage, logDest string) {
	sender := dataMessage.SenderId.UUID
	logmsg := fmt.Sprintf("%v -> %v: %v", sender,
		neuron.NodeId.UUID, dataMessage)
	logg.LogTo(logDest, logmsg)
}

func (neuron *Neuron) createEmptyWeightedInputs() {
	neuron.weightedInputs = createEmptyWeightedInputs(neuron.Inbound)
}

func (neuron *Neuron) closeChannels() {
	neuron.Closing = nil
	neuron.DataChan = nil
}

func logPreSend(senderNodeId *NodeId, receiverNodeId *NodeId, dataMessage *DataMessage) {
	logSend(senderNodeId, receiverNodeId, dataMessage, "NODE_PRE_SEND")
}

func logPostSend(senderNodeId *NodeId, receiverNodeId *NodeId, dataMessage *DataMessage) {
	logSend(senderNodeId, receiverNodeId, dataMessage, "NODE_POST_SEND")
}

func logWeights(neuron *Neuron) {
	for _, inboundConnection := range neuron.Inbound {
		logmsg := fmt.Sprintf("%v -> %v weights: %v", inboundConnection.NodeId.UUID, neuron.NodeId.UUID, inboundConnection.Weights)
		logg.LogTo("NODE_STATE", logmsg)
		logmsg = fmt.Sprintf("%v bias: %v", neuron.NodeId.UUID, neuron.Bias)
		logg.LogTo("NODE_STATE", logmsg)
	}
}

func logSend(senderNodeId *NodeId, receiverNodeId *NodeId, dataMessage *DataMessage, logDest string) {
	logmsg := fmt.Sprintf("%v -> %v: %v", senderNodeId.UUID,
		receiverNodeId.UUID, dataMessage)
	logg.LogTo(logDest, logmsg)
}

func logRecurrentSend(neuronNodeId *NodeId, dataMessage *DataMessage) {
	logmsg := fmt.Sprintf("%v -> %v (recurrent send): %v", neuronNodeId.UUID,
		neuronNodeId.UUID, dataMessage)
	logg.LogTo("NODE_PRE_SEND", logmsg)
}

func logRecurrentRecv(neuronNodeId *NodeId, dataMessage *DataMessage) {
	logmsg := fmt.Sprintf("%v -> %v (recurrent recv): %v", neuronNodeId.UUID,
		neuronNodeId.UUID, dataMessage)
	logg.LogTo("NODE_POST_RECV", logmsg)
}
