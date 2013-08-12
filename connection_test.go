package neurgo

import (
	"github.com/couchbaselabs/go.assert"
	"testing"
)

func TestCreateEmptyWeightedInputs(t *testing.T) {

	nodeId_1 := &NodeId{UUID: "node-1", NodeType: NEURON}
	nodeId_2 := &NodeId{UUID: "node-2", NodeType: NEURON}

	weights_1 := []float64{1, 1, 1, 1, 1}
	weights_2 := []float64{1}

	inboundConnection1 := &InboundConnection{
		NodeId:  nodeId_1,
		Weights: weights_1,
	}
	inboundConnection2 := &InboundConnection{
		NodeId:  nodeId_2,
		Weights: weights_2,
	}

	inbound := []*InboundConnection{
		inboundConnection1,
		inboundConnection2,
	}

	weightedInputs := createEmptyWeightedInputs(inbound)
	assert.Equals(t, len(inbound), len(weightedInputs))
	assert.Equals(t, inbound[0].NodeId.UUID, weightedInputs[0].senderNodeUUID)

}

func TestConnections(t *testing.T) {

	shouldReInit := false

	sensorNodeId := NewSensorId("sensor", 0.0)
	hiddenNeuron1NodeId := NewNeuronId("hidden-neuron1", 0.25)
	hiddenNeuron2NodeId := NewNeuronId("hidden-neuron2", 0.25)
	outputNeuronNodeIde := NewNeuronId("output-neuron", 0.35)

	actuatorNodeId := NewActuatorId("actuator", 0.5)

	hiddenNeuron1 := &Neuron{
		ActivationFunction: EncodableSigmoid(),
		NodeId:             hiddenNeuron1NodeId,
		Bias:               -30,
	}
	hiddenNeuron1.Init(shouldReInit)

	hiddenNeuron2 := &Neuron{
		ActivationFunction: EncodableSigmoid(),
		NodeId:             hiddenNeuron2NodeId,
		Bias:               10,
	}
	hiddenNeuron2.Init(shouldReInit)

	outputNeuron := &Neuron{
		ActivationFunction: EncodableSigmoid(),
		NodeId:             outputNeuronNodeIde,
		Bias:               -10,
	}
	outputNeuron.Init(shouldReInit)

	sensor := &Sensor{
		NodeId:       sensorNodeId,
		VectorLength: 2,
	}
	sensor.Init(shouldReInit)

	actuator := &Actuator{
		NodeId:       actuatorNodeId,
		VectorLength: 1,
	}
	actuator.Init(shouldReInit)

	sensor.ConnectOutbound(hiddenNeuron1)
	hiddenNeuron1.ConnectInboundWeighted(sensor, []float64{20, 20})

	sensor.ConnectOutbound(hiddenNeuron2)
	hiddenNeuron2.ConnectInboundWeighted(sensor, []float64{-20, -20})

	assert.Equals(t, len(sensor.Outbound), 2)
	assert.Equals(t, len(hiddenNeuron1.Inbound), 1)
	assert.Equals(t, len(hiddenNeuron2.Inbound), 1)

	hiddenNeuron1.ConnectOutbound(outputNeuron)
	outputNeuron.ConnectInboundWeighted(hiddenNeuron1, []float64{20})

	hiddenNeuron2.ConnectOutbound(outputNeuron)
	outputNeuron.ConnectInboundWeighted(hiddenNeuron2, []float64{20})

	assert.Equals(t, len(hiddenNeuron1.Outbound), 1)
	assert.Equals(t, len(hiddenNeuron2.Outbound), 1)
	assert.Equals(t, len(outputNeuron.Inbound), 2)

	outputNeuron.ConnectOutbound(actuator)
	actuator.ConnectInbound(outputNeuron)
	assert.Equals(t, len(outputNeuron.Outbound), 1)
	assert.Equals(t, len(actuator.Inbound), 1)

}
