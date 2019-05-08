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

package rabbitpubsub_test

import (
	"context"
	"log"

	"github.com/streadway/amqp"
	"github.com/eliben/gocdkx/pubsub"
	"github.com/eliben/gocdkx/pubsub/rabbitpubsub"
)

func ExampleOpenTopic() {
	// This example is used in https://github.com/eliben/gocdkx/howto/pubsub/publish/#rabbitmq-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	rabbitConn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatal(err)
	}
	defer rabbitConn.Close()
	topic := rabbitpubsub.OpenTopic(rabbitConn, "myexchange", nil)
	defer topic.Shutdown(ctx)
}

func ExampleOpenSubscription() {
	// This example is used in https://github.com/eliben/gocdkx/howto/pubsub/subscribe/#rabbitmq-ctor

	// Variables set up elsewhere:
	ctx := context.Background()

	rabbitConn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatal(err)
	}
	defer rabbitConn.Close()
	subscription := rabbitpubsub.OpenSubscription(rabbitConn, "myqueue", nil)
	defer subscription.Shutdown(ctx)
}

func Example_openTopic() {
	// This example is used in https://github.com/eliben/gocdkx/howto/pubsub/publish/#rabbitmq

	// import _ "github.com/eliben/gocdkx/pubsub/rabbitpubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	// OpenTopic creates a *pubsub.Topic from a URL.
	// This URL will Dial the RabbitMQ server at the URL in the environment
	// variable RABBIT_SERVER_URL and open the exchange "myexchange".
	topic, err := pubsub.OpenTopic(ctx, "rabbit://myexchange")
	if err != nil {
		log.Fatal(err)
	}
	defer topic.Shutdown(ctx)
}

func Example_openSubscription() {
	// This example is used in https://github.com/eliben/gocdkx/howto/pubsub/subscribe/#rabbitmq

	// import _ "github.com/eliben/gocdkx/pubsub/rabbitpubsub"

	// Variables set up elsewhere:
	ctx := context.Background()

	// OpenSubscription creates a *pubsub.Subscription from a URL.
	// This URL will Dial the RabbitMQ server at the URL in the environment
	// variable RABBIT_SERVER_URL and open the queue "myqueue".
	subscription, err := pubsub.OpenSubscription(ctx, "rabbit://myqueue")
	if err != nil {
		log.Fatal(err)
	}
	defer subscription.Shutdown(ctx)
}
