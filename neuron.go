package neurgo

import (
	"encoding/json"
	"fmt"
	"github.com/proxypoke/vector"
	"log"
	"sync"
)

type ActivationFunction func(float64) float64

type Neuron struct {
	NodeId             *NodeId
	Bias               float64
	Inbound            []*InboundConnection
	Outbound           []*OutboundConnection
	Closing            chan chan bool
	DataChan           chan *DataMessage
	ActivationFunction ActivationFunction
	wg                 sync.WaitGroup
}

func (neuron *Neuron) MarshalJSON() ([]byte, error) {
	return json.Marshal(
		struct {
			NodeId   *NodeId
			Bias     float64
			Inbound  []*InboundConnection
			Outbound []*OutboundConnection
		}{
			NodeId:   neuron.NodeId,
			Bias:     neuron.Bias,
			Inbound:  neuron.Inbound,
			Outbound: neuron.Outbound,
		})
}

func (neuron *Neuron) Run() {

	defer neuron.wg.Done()

	neuron.checkRunnable()

	neuron.sendEmptySignalRecurrentOutbound()

	weightedInputs := createEmptyWeightedInputs(neuron.Inbound)

	closed := false

	for {

		log.Printf("Neuron %v select().  datachan: %v", neuron, neuron.DataChan)

		select {
		case responseChan := <-neuron.Closing:
			closed = true
			responseChan <- true
			break // TODO: do we need this for anything??
		case dataMessage := <-neuron.DataChan:
			recordInput(weightedInputs, dataMessage)
		}

		if closed {
			neuron.Closing = nil
			neuron.DataChan = nil
			break
		}

		if receiveBarrierSatisfied(weightedInputs) {

			log.Printf("Neuron %v received inputs: %v", neuron, weightedInputs)
			scalarOutput := neuron.computeScalarOutput(weightedInputs)

			dataMessage := &DataMessage{
				SenderId: neuron.NodeId,
				Inputs:   []float64{scalarOutput},
			}

			neuron.scatterOutput(dataMessage)

			weightedInputs = createEmptyWeightedInputs(neuron.Inbound)

		} else {
			log.Printf("Neuron %v receive barrier not satisfied", neuron)
		}

	}

}

func (neuron *Neuron) String() string {
	return fmt.Sprintf("%v", neuron.NodeId)
}

func (neuron *Neuron) ConnectOutbound(connectable OutboundConnectable) {
	if neuron.Outbound == nil {
		neuron.Outbound = make([]*OutboundConnection, 0)
	}
	connection := &OutboundConnection{
		NodeId:   connectable.nodeId(),
		DataChan: connectable.dataChan(),
	}
	neuron.Outbound = append(neuron.Outbound, connection)
}

func (neuron *Neuron) ConnectInboundWeighted(connectable InboundConnectable, weights []float64) {
	if neuron.Inbound == nil {
		neuron.Inbound = make([]*InboundConnection, 0)
	}
	connection := &InboundConnection{
		NodeId:  connectable.nodeId(),
		Weights: weights,
	}
	neuron.Inbound = append(neuron.Inbound, connection)

}

// In order to prevent deadlock, any neurons we have recurrent outbound
// connections to must be "primed" by sending an empty signal.  A recurrent
// outbound connection simply means that it's a connection to ourself or
// to a neuron in a previous (eg, to the left) layer.  If we didn't do this,
// that previous neuron would be waiting forever for a signal that will
// never come, because this neuron wouldn't fire until it got a signal.
func (neuron *Neuron) sendEmptySignalRecurrentOutbound() {

	recurrentConnections := neuron.recurrentOutboundConnections()
	log.Printf("Neuron %v recurrent connections: %v", neuron, recurrentConnections)
	for _, recurrentConnection := range recurrentConnections {

		inputs := []float64{0}
		dataMessage := &DataMessage{
			SenderId: neuron.NodeId,
			Inputs:   inputs,
		}
		log.Printf("Neuron %v sending data %v to recurrent outbound: %v", neuron, dataMessage, recurrentConnection)
		recurrentConnection.DataChan <- dataMessage
	}

}

// Find the subset of outbound connections which are "recurrent" - meaning
// that the connection is to this neuron itself, or to a neuron in a previous
// (eg, to the left) layer.
func (neuron *Neuron) recurrentOutboundConnections() []*OutboundConnection {
	result := make([]*OutboundConnection, 0)
	for _, outboundConnection := range neuron.Outbound {
		if neuron.isConnectionRecurrent(outboundConnection) {
			result = append(result, outboundConnection)
		}
	}
	return result
}

// a connection is considered recurrent if it has a connection
// to itself or to a node in a previous layer.  Previous meaning
// if you look at a feedforward from left to right, with the input
// layer being on the far left, and output layer on the far right,
// then any layer to the left is considered previous.
func (neuron *Neuron) isConnectionRecurrent(connection *OutboundConnection) bool {
	if connection.NodeId.LayerIndex <= neuron.NodeId.LayerIndex {
		return true
	}
	return false
}

func (neuron *Neuron) scatterOutput(dataMessage *DataMessage) {
	for _, outboundConnection := range neuron.Outbound {
		dataChan := outboundConnection.DataChan
		log.Printf("Neuron %v scatter %v to: %v", neuron, dataMessage, outboundConnection)
		dataChan <- dataMessage
	}
}

func (neuron *Neuron) Init() {
	if neuron.Closing == nil {
		neuron.Closing = make(chan chan bool)
	} else {
		msg := "Warn: %v Init() called, but already had closing channel"
		log.Printf(msg, neuron)
	}

	if neuron.DataChan == nil {
		neuron.DataChan = make(chan *DataMessage, len(neuron.Inbound))
	} else {
		msg := "Warn: %v Init() called, but already had data channel"
		log.Printf(msg, neuron)
	}
	neuron.wg.Add(1) // TODO: make sure Init() not called twice!
}

func (neuron *Neuron) Shutdown() {

	closingResponse := make(chan bool)
	neuron.Closing <- closingResponse
	response := <-closingResponse
	if response != true {
		log.Panicf("Got unexpected response on closing channel")
	}

	neuron.wg.Wait()
}

func (neuron *Neuron) checkRunnable() {

	if neuron.NodeId == nil {
		msg := fmt.Sprintf("not expecting neuron.NodeId to be nil")
		panic(msg)
	}

	if neuron.Inbound == nil {
		msg := fmt.Sprintf("not expecting neuron.Inbound to be nil")
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

}

func (neuron *Neuron) computeScalarOutput(weightedInputs []*weightedInput) float64 {
	output := neuron.weightedInputDotProductSum(weightedInputs)
	output += neuron.Bias
	output = neuron.ActivationFunction(output)
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
