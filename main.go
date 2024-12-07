package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs/checkpoints"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

func main() {
	sendMsgs()
	getMsgs()
}

func sendMsgs() {
	// create an Event Hubs producer client using a connection string to the namespace and the event hub
	producerClient, err := azeventhubs.NewProducerClientFromConnectionString("Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;", "eh1", nil)

	if err != nil {
		panic(err)
	}

	defer producerClient.Close(context.TODO())

	// create sample events
	events := createEventsForSample()

	// create a batch object and add sample events to the batch
	newBatchOptions := &azeventhubs.EventDataBatchOptions{}

	batch, err := producerClient.NewEventDataBatch(context.TODO(), newBatchOptions)

	if err != nil {
		panic(err)
	}

	for i := 0; i < len(events); i++ {
		err = batch.AddEventData(events[i], nil)

		if err != nil {
			panic(err)
		}
	}

	// send the batch of events to the event hub
	err = producerClient.SendEventDataBatch(context.TODO(), batch, nil)

	if err != nil {
		panic(err)
	}
}

func createEventsForSample() []*azeventhubs.EventData {
	return []*azeventhubs.EventData{
		{
			Body: []byte("hello"),
		},
		{
			Body: []byte("world"),
		},
	}
}

func getMsgs() {

	// create a container client using a connection string and container name
	checkClient, err := container.NewClientFromConnectionString("AZURE STORAGE CONNECTION STRING", "CONTAINER NAME", nil)

	if err != nil {
		panic(err)
	}

	// create a checkpoint store that will be used by the event hub
	checkpointStore, err := checkpoints.NewBlobStore(checkClient, nil)

	if err != nil {
		panic(err)
	}

	// create a consumer client using a connection string to the namespace and the event hub
	consumerClient, err := azeventhubs.NewConsumerClientFromConnectionString("NAMESPACE CONNECTION STRING", "EVENT HUB NAME", azeventhubs.DefaultConsumerGroup, nil)

	if err != nil {
		panic(err)
	}

	defer consumerClient.Close(context.TODO())

	// create a processor to receive and process events
	processor, err := azeventhubs.NewProcessor(consumerClient, checkpointStore, nil)

	if err != nil {
		panic(err)
	}

	//  for each partition in the event hub, create a partition client with processEvents as the function to process events
	dispatchPartitionClients := func() {
		for {
			partitionClient := processor.NextPartitionClient(context.TODO())

			if partitionClient == nil {
				break
			}

			go func() {
				if err := processEvents(partitionClient); err != nil {
					panic(err)
				}
			}()
		}
	}

	// run all partition clients
	go dispatchPartitionClients()

	processorCtx, processorCancel := context.WithCancel(context.TODO())
	defer processorCancel()

	if err := processor.Run(processorCtx); err != nil {
		panic(err)
	}
}

func processEvents(partitionClient *azeventhubs.ProcessorPartitionClient) error {
	defer closePartitionResources(partitionClient)
	for {
		receiveCtx, receiveCtxCancel := context.WithTimeout(context.TODO(), time.Minute)
		events, err := partitionClient.ReceiveEvents(receiveCtx, 100, nil)
		receiveCtxCancel()

		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		fmt.Printf("Processing %d event(s)\n", len(events))

		for _, event := range events {
			fmt.Printf("Event received with body %v\n", string(event.Body))
		}

		if len(events) != 0 {
			if err := partitionClient.UpdateCheckpoint(context.TODO(), events[len(events)-1], nil); err != nil {
				return err
			}
		}
	}
}

func closePartitionResources(partitionClient *azeventhubs.ProcessorPartitionClient) {
	defer partitionClient.Close(context.TODO())
}
