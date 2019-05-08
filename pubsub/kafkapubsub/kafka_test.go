// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kafkapubsub // import "github.com/eliben/gocdkx/pubsub/kafkapubsub"

// To run these tests against a real Kafka server, run localkafka.sh.
// See https://github.com/spotify/docker-kafka for more on the docker container
// that the script runs.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/eliben/gocdkx/pubsub"
	"github.com/eliben/gocdkx/pubsub/driver"
	"github.com/eliben/gocdkx/pubsub/drivertest"
)

var (
	localBrokerAddrs = []string{"localhost:9092"}
	// Makes OpenSubscription wait ~forever until the subscriber has joined the
	// ConsumerGroup. Messages sent to the topic before the subscriber has joined
	// won't be received.
	subscriptionOptions = &SubscriptionOptions{WaitForJoin: 24 * time.Hour}
)

type harness struct {
	uniqueID  int
	numSubs   uint32
	numTopics uint32
}

var checkOnce sync.Once
var kafkaRunning bool

func localKafkaRunning() bool {
	checkOnce.Do(func() {
		_, err := sarama.NewClient(localBrokerAddrs, MinimalConfig())
		if err == nil {
			kafkaRunning = true
		}
	})
	return kafkaRunning
}

func newHarness(ctx context.Context, t *testing.T) (drivertest.Harness, error) {
	if !localKafkaRunning() {
		t.Skip("No local Kafka running, see pubsub/kafkapubsub/localkafka.sh")
	}
	return &harness{uniqueID: rand.Int()}, nil
}

func createKafkaTopic(topicName string) (func(), error) {
	// Create the topic.
	config := MinimalConfig()
	admin, err := sarama.NewClusterAdmin(localBrokerAddrs, config)
	if err != nil {
		return func() {}, err
	}
	close1 := func() { admin.Close() }

	topicDetail := &sarama.TopicDetail{
		NumPartitions:     1,
		ReplicationFactor: 1,
	}
	if err := admin.CreateTopic(topicName, topicDetail, false); err != nil {
		return close1, err
	}
	close2 := func() {
		admin.DeleteTopic(topicName)
		close1()
	}
	return close2, nil
}

func (h *harness) CreateTopic(ctx context.Context, testName string) (driver.Topic, func(), error) {
	topicName := fmt.Sprintf("%s-topic-%d-%d", sanitize(testName), h.uniqueID, atomic.AddUint32(&h.numTopics, 1))
	cleanup, err := createKafkaTopic(topicName)
	if err != nil {
		return nil, cleanup, err
	}

	// Open it.
	dt, err := openTopic(localBrokerAddrs, MinimalConfig(), topicName, nil)
	if err != nil {
		return nil, cleanup, err
	}
	return dt, cleanup, nil
}

func (h *harness) MakeNonexistentTopic(ctx context.Context) (driver.Topic, error) {
	return openTopic(localBrokerAddrs, MinimalConfig(), "nonexistent-topic", nil)
}

func (h *harness) CreateSubscription(ctx context.Context, dt driver.Topic, testName string) (driver.Subscription, func(), error) {
	groupID := fmt.Sprintf("%s-sub-%d-%d", sanitize(testName), h.uniqueID, atomic.AddUint32(&h.numSubs, 1))
	ds, err := openSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{dt.(*topic).topicName}, subscriptionOptions)
	if err != nil {
		return nil, nil, err
	}
	return ds, func() {}, nil
}

func (h *harness) MakeNonexistentSubscription(ctx context.Context) (driver.Subscription, error) {
	return openSubscription(localBrokerAddrs, MinimalConfig(), "unused-group", []string{"nonexistent-topic"}, subscriptionOptions)
}

func (h *harness) Close() {}

func (h *harness) MaxBatchSizes() (int, int) { return sendBatcherOpts.MaxBatchSize, 0 }

func TestConformance(t *testing.T) {
	asTests := []drivertest.AsTest{asTest{}}
	drivertest.RunConformanceTests(t, newHarness, asTests)
}

type asTest struct{}

func (asTest) Name() string {
	return "kafka"
}

func (asTest) TopicCheck(topic *pubsub.Topic) error {
	var sp sarama.SyncProducer
	if !topic.As(&sp) {
		return fmt.Errorf("cast failed for %T", sp)
	}
	return nil
}

func (asTest) SubscriptionCheck(sub *pubsub.Subscription) error {
	var cg sarama.ConsumerGroup
	if !sub.As(&cg) {
		return fmt.Errorf("cast failed for %T", cg)
	}
	var cgs sarama.ConsumerGroupSession
	if !sub.As(&cgs) {
		return fmt.Errorf("cast failed for %T", cgs)
	}
	return nil
}

func (asTest) TopicErrorCheck(t *pubsub.Topic, err error) error {
	var pe sarama.ProducerErrors
	if !t.ErrorAs(err, &pe) {
		return fmt.Errorf("failed to convert %v (%T)", err, err)
	}
	return nil
}

func (asTest) SubscriptionErrorCheck(s *pubsub.Subscription, err error) error {
	var ke sarama.KError
	if !s.ErrorAs(err, &ke) {
		return fmt.Errorf("failed to convert %v (%T)", err, err)
	}
	return nil
}

func (asTest) MessageCheck(m *pubsub.Message) error {
	var cm *sarama.ConsumerMessage
	if !m.As(&cm) {
		return fmt.Errorf("cast failed for %T", cm)
	}
	return nil
}

func (asTest) BeforeSend(as func(interface{}) bool) error {
	var pm *sarama.ProducerMessage
	if !as(&pm) {
		return fmt.Errorf("cast failed for %T", &pm)
	}
	return nil
}

// TestKafkaKey tests sending/receiving a message with the Kafka message key set.
func TestKafkaKey(t *testing.T) {
	if !localKafkaRunning() {
		t.Skip("No local Kafka running, see pubsub/kafkapubsub/localkafka.sh")
	}
	const (
		keyName  = "kafkakey"
		keyValue = "kafkakeyvalue"
	)
	uniqueID := rand.Int()
	ctx := context.Background()

	topicName := fmt.Sprintf("%s-topic-%d", sanitize(t.Name()), uniqueID)
	topicCleanup, err := createKafkaTopic(topicName)
	defer topicCleanup()
	if err != nil {
		t.Fatal(err)
	}
	topic, err := OpenTopic(localBrokerAddrs, MinimalConfig(), topicName, &TopicOptions{KeyName: keyName})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := topic.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	groupID := fmt.Sprintf("%s-sub-%d", sanitize(t.Name()), uniqueID)
	subOpts := *subscriptionOptions
	subOpts.KeyName = keyName
	sub, err := OpenSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{topicName}, &subOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sub.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	m := &pubsub.Message{
		Metadata: map[string]string{
			"foo":   "bar",
			keyName: keyValue,
		},
		Body: []byte("hello world"),
		BeforeSend: func(as func(interface{}) bool) error {
			// Verify that the Key field was set correctly on the outgoing Kafka
			// message.
			var pm *sarama.ProducerMessage
			if !as(&pm) {
				return errors.New("failed to convert to ProducerMessage")
			}
			gotKeyBytes, err := pm.Key.Encode()
			if err != nil {
				return fmt.Errorf("failed to Encode Kafka Key: %v", err)
			}
			if gotKey := string(gotKeyBytes); gotKey != keyValue {
				return errors.New("Kafka key wasn't set appropriately")
			}
			return nil
		},
	}
	err = topic.Send(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	// The test will hang here if the message isn't available, so use a shorter timeout.
	ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	got, err := sub.Receive(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	got.Ack()

	m.BeforeSend = nil // don't expect this in the received message
	if diff := cmp.Diff(got, m, cmpopts.IgnoreUnexported(pubsub.Message{})); diff != "" {
		t.Errorf("got\n%v\nwant\n%v\ndiff\n%v", got, m, diff)
	}

	// Verify that Key was set in the received Kafka message via As.
	var cm *sarama.ConsumerMessage
	if !got.As(&cm) {
		t.Fatal("failed to get message As ConsumerMessage")
	}
	if gotKey := string(cm.Key); gotKey != keyValue {
		t.Errorf("got key %q want %q", gotKey, keyValue)
	}
}

func sanitize(testName string) string {
	return strings.Replace(testName, "/", "_", -1)
}

func BenchmarkKafka(b *testing.B) {
	ctx := context.Background()
	uniqueID := rand.Int()

	// Create the topic.
	topicName := fmt.Sprintf("%s-topic-%d", b.Name(), uniqueID)
	cleanup, err := createKafkaTopic(topicName)
	defer cleanup()
	if err != nil {
		b.Fatal(err)
	}

	topic, err := OpenTopic(localBrokerAddrs, MinimalConfig(), topicName, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer topic.Shutdown(ctx)

	groupID := fmt.Sprintf("%s-subscription-%d", b.Name(), uniqueID)
	sub, err := OpenSubscription(localBrokerAddrs, MinimalConfig(), groupID, []string{topicName}, subscriptionOptions)
	if err != nil {
		b.Fatal(err)
	}
	defer sub.Shutdown(ctx)

	drivertest.RunBenchmarks(b, topic, sub)
}

func fakeConnectionStringInEnv() func() {
	oldEnvVal := os.Getenv("KAFKA_BROKERS")
	os.Setenv("KAFKA_BROKERS", "localhost:10000")
	return func() {
		os.Setenv("KAFKA_BROKERS", oldEnvVal)
	}
}

func TestOpenTopicFromURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK, but still error because broker doesn't exist.
		{"kafka://mytopic", true},
		// Invalid parameter.
		{"kafka://mytopic?param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := pubsub.OpenTopic(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}

func TestOpenSubscriptionFromURL(t *testing.T) {
	cleanup := fakeConnectionStringInEnv()
	defer cleanup()

	tests := []struct {
		URL     string
		WantErr bool
	}{
		// OK, but still error because broker doesn't exist.
		{"kafka://mygroup?topic=mytopic", true},
		// Invalid parameter.
		{"kafka://mygroup?topic=mytopic&param=value", true},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := pubsub.OpenSubscription(ctx, test.URL)
		if (err != nil) != test.WantErr {
			t.Errorf("%s: got error %v, want error %v", test.URL, err, test.WantErr)
		}
	}
}
