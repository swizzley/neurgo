package neurgo

import (
	"testing"
	"github.com/couchbaselabs/go.assert"
	"sync"
	"time"
)


func TestConnectBidirectional(t *testing.T) {

	// create nodes
	neuron := &Node{processor: &Neuron{}}
	sensor := &Node{processor: &Sensor{}}

	// give names
	neuron.Name = "neuron"
	sensor.Name = "sensor"

	// make connection
	weights := []float64{20,20,20,20,20}
	sensor.ConnectBidirectionalWeighted(neuron, weights)

	// make sure the reverse connection points back to correct node type
	sensorTypeCheck := neuron.inbound[0].other.processor.(*Sensor)
	assert.Equals(t, sensor, sensorTypeCheck)

	// assert that it worked
	assert.Equals(t, len(sensor.outbound), 1)
	assert.Equals(t, len(neuron.inbound), 1)
	assert.True(t, neuron.inbound[0].channel != nil)
	assert.True(t, sensor.outbound[0].channel != nil)
	assert.Equals(t, len(neuron.inbound[0].weights), len(weights))
	assert.Equals(t, neuron.inbound[0].weights[0], weights[0])

	// make a new node and connect it
	actuator := &Node{processor: &Actuator{}}
	neuron.ConnectBidirectional(actuator)

	// make sure the reverse connection points back to correct node type
	neuronTypeCheck := actuator.inbound[0].other.processor.(*Neuron)
	assert.Equals(t, neuron, neuronTypeCheck)

	// assert that it worked
	assert.Equals(t, len(neuron.outbound), 1)
	assert.Equals(t, len(actuator.inbound), 1)
	assert.Equals(t, len(actuator.inbound[0].weights), 0)

}


func TestRemoveConnection(t *testing.T) {

	// create network nodes
	neuronProcessor1 := &Neuron{Bias: 10, ActivationFunction: identity_activation}  
	neuronProcessor2 := &Neuron{Bias: 10, ActivationFunction: identity_activation}
	neuron1 := &Node{Name: "neuron1", processor: neuronProcessor1}
	neuron2 := &Node{Name: "neuron2", processor: neuronProcessor2}
	sensor := &Node{Name: "sensor", processor: &Sensor{}}

	// connect nodes together 
	weights := []float64{20,20,20,20,20}
	sensor.ConnectBidirectionalWeighted(neuron1, weights)
	sensor.ConnectBidirectionalWeighted(neuron2, weights)

	// remove connections
	neuron1.inbound = removeConnection(neuron1.inbound, 0) 
	sensor.outbound = removeConnection(sensor.outbound, 0) 

	// assert that it worked
	assert.Equals(t, len(neuron1.inbound), 0)
	assert.Equals(t, len(neuron2.inbound), 1)
	assert.Equals(t, len(sensor.outbound), 1)
	assert.Equals(t, sensor.outbound[0].channel, neuron2.inbound[0].channel)

}

func TestRemoveConnectionFromRunningNeuron(t *testing.T) {

	// create nodes
	sensor1 := &Node{Name: "sensor1", processor: &Sensor{}}
	sensor2 := &Node{Name: "sensor2", processor: &Sensor{}}
	neuronProcessor := &Neuron{Bias: 10, ActivationFunction: identity_activation}
	neuron := &Node{Name: "neuron", processor: neuronProcessor}

	// connect nodes together
	weights := []float64{20}
	sensor1.ConnectBidirectionalWeighted(neuron, weights)
	sensor2.ConnectBidirectionalWeighted(neuron, weights)

	// basic sanity check, send two inputs to neuron inbound channels
	// and verify that weightedInputs() returns both inputs
	go func() {
		sensor1.outbound[0].channel <- []float64{0}
	}()
	go func() {
		sensor2.outbound[0].channel <- []float64{0}
	}()
	weightedInputs := neuronProcessor.weightedInputs(neuron)
	assert.Equals(t, len(weightedInputs), 2)
	
	// close one channel while a neuron is reading from
	// both inbound connections, make sure it returns one value
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Add(1)
	
	go func() {
		weightedInputs := neuronProcessor.weightedInputs(neuron)
		assert.Equals(t, len(weightedInputs), 1)
		wg.Done() 
	}()

	go func() {
		
		// need to sleep so that we can be sure that the other go-routine 
		// is blocked on the channel read of its inbound channels
		time.Sleep(0.1 * 1e9)
		
		sensor1.DisconnectBidirectional(neuron)
		sensor2.outbound[0].channel <- []float64{0}
		wg.Done() 
	}()

	wg.Wait()


}

func TestRemoveConnectionFromRunningActuator(t *testing.T) {

	// create nodes
	neuronProcessor1 := &Neuron{Bias: 10, ActivationFunction: identity_activation}  
	neuronProcessor2 := &Neuron{Bias: 10, ActivationFunction: identity_activation}
	neuron1 := &Node{Name: "neuron1", processor: neuronProcessor1}
	neuron2 := &Node{Name: "neuron2", processor: neuronProcessor2}
	actuatorProcessor := &Actuator{}
	actuator := &Node{Name: "actuator", processor: actuatorProcessor}

	// connect nodes together
	neuron1.ConnectBidirectional(actuator)
	neuron2.ConnectBidirectional(actuator)

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Add(1)
	
	go func() {
		inputs := actuatorProcessor.gatherInputs(actuator)
		assert.Equals(t, len(inputs), 1)
		wg.Done() 
	}()

	go func() {
		
		// need to sleep so that we can be sure that the other go-routine 
		// is blocked on the channel read of its inbound channels
		time.Sleep(0.1 * 1e9)
		
		neuron1.DisconnectBidirectional(actuator)
		neuron2.outbound[0].channel <- []float64{0}
		wg.Done() 
	}()

	wg.Wait()


}


func identity_activation(x float64) float64 {
	return x
}
