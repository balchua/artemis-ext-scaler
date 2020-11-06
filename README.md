# Keda External Scaler with ActiveMQ Artemis
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fbalchua%2Fartemis-ext-scaler.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Fbalchua%2Fartemis-ext-scaler?ref=badge_shield)
[![](https://github.com/balchua/artemis-ext-scaler/workflows/Build%20metrics-provider/badge.svg)](https://github.com/balchua/artemis-ext-scaler/actions)

This is a demonstration on how to use KEDA's external scaler to monitor [ActiveMQ Artemis](https://activemq.apache.org/components/artemis/) Queue.

The Keda external scaler calls metrics-provider GRPC server which will collect the metrics from ActiveMQ Artemis.

***ActiveMQ Artemis is now included as part of the KEDA internal scaler***

**Note: use your own docker repository while building the project.**

## Pre-requisites:

* Kubernetes cluster
* Keda installed in the cluster.
* skaffold - used to build the images
* maven - use to build the java consumer and producer project.

## Code organization

* consumer - a simple Springboot application which consumes messages from ActiveMQ Artemis.  The queue name used is `test`.  The `consumerWindowSize` is also set to `0` so that it will not buffer the messages to one consumer.
* producer - a simple Springboot application which pumps messages to the queue `test`
* metrics-provider - a Go application which serves as the GRPC server to serve the Keda External Scaler.  It queries ActiveMQ Artemis `MessageCount` using the built -in jolokia endpoint exposed by Artemis.
* k8s-manifest - everything Kubernetes
  
### Building the metrics provider for external scaler

You can get the `proto` file from the Keda [github](https://github.com/kedacore/keda/blob/master/pkg/scalers/externalscaler/externalscaler.proto).

This project already contains the [`externalscaler.proto`](metrics-provider/externalscaler/proto/externalscaler.proto)

### Use protoc to autogenerate the Proto codes.

`protoc -I externalscaler/ externalscaler/proto/externalscaler.proto --go_out=plugins=grpc:externalscaler`

**Note: We use kaniko in-cluster builder**

#### Setup kaniko registry access secret

`kubectl -n artemis create secret generic regcred --from-file $HOME/.docker/config.json`

```shell
$ skaffold run -p metrics-provider
```

### External Scaler as ActiveMQ Artemis sidecar

The docker image used taken from [vromero/activemq-artemis-docker](https://github.com/vromero/activemq-artemis-docker) uses the `hostname` as its broker name, in order to avoid hardcoding the broker name, the metrics provider is deployed as a _sidecar_ to ActiveMQ Artemis.

Added to the file [`k8s-manifest/artemis/deployment.yaml`](k8s-manifest/artemis/deployment.yaml)

```yaml
  containers:
  - name: artemis-activemq-metrics-provider
    image: docker.io/balchu/artemis-ext-scaler:1.0.0
    args: ["--port","5050","--broker","$(POD_NAME)", "--user", "$(ARTEMIS_USERNAME)","--password","${ARTEMIS_PASSWOORD)"]
    imagePullPolicy: Always
    resources:
      requests:
        cpu: 100m
        memory: 10Mi   
```            

### Build and deploy the consumer.

Using skaffold and jib, simply execute the command below.

`skaffold run -p consumer`

Please make sure that you use your docker repository.

### Deploy the External Scaler manifest

Now its time to setup the KEDA's external scaler.

`kubectl apply -f k8s-manifest/externalscaler_scaledobject.yaml`

The file looks like this.

```yaml
apiVersion: keda.k8s.io/v1alpha1
kind: ScaledObject
metadata:
  name: artemis-scaledobject
  namespace: artemis
  labels:
    deploymentName: artemis-consumer
spec:
  pollingInterval: 10   # Optional. Default: 30 seconds
  cooldownPeriod: 100  # Optional. Default: 300 seconds
  minReplicaCount: 0   # Optional. Default: 0
  maxReplicaCount: 30  # Optional. Default: 100  
  scaleTargetRef:
    deploymentName: artemis-consumer
  triggers:
  - type: external
    metadata:
      scalerAddress: artemis-activemq.artemis:5050
      queueLength: "10"
      brokerAddress: "test"
      queueName: "test"
```      

Where:
* `scalerAddress`: is the location of the GRPC metrics provider host and port.
* `queueLength`: the target queue length.
* `brokerAddress`: An address represents a messaging endpoint. Within the configuration, a typical address is given a unique name, 0 or more queues, and a routing type.
* `queueName`: the name of queue to monitor.

Before pumping in messages, check the HPA.

### Check the HPA

```shell

kubectl -n artemis get hpa
NAME                        REFERENCE                     TARGETS              MINPODS   MAXPODS   REPLICAS   AGE
keda-hpa-artemis-consumer   Deployment/artemis-consumer   <unknown>/20 (avg)   1         30        0          58m

```

At this point,HPA doesn't have the `TargetAverageValue` to scale up or down the pods.  This can be observed by the `<unknown>/20(avg)`


### Start the producer

The producer is a simple Springboot application.

If you are going to run the Springboot application using your IDE, make sure that you point to the host and port of the ActiveMQ Artemis.

Check the file [application.yml](producer/src/main/resources/application.yml)

As an example:

```yaml
spring:
  artemis:
    mode: native
    host: ${ARTEMIS_SERVER_HOST:10.152.183.227}
    port: ${ARTEMIS_SERVER_PORT:61616}
    user: ${ARTEMIS_USERNAME:artemis}
    password: ${ARTEMIS_PASSWORD:artemis}
```

In the Class [App.java](producer/src/main/java/org/bal/starter/App.java)

You can modify how much messages you want to send to the broker.  In the example below, the program is pushing 10000 messages to the broker, with a delay of 200 milliseonds.

```java
public void run(String... args) throws Exception {
	for (int i = 0; i < 10000; i++){
		producer.send("Message is: " + System.currentTimeMillis());
		sleep(200);
   }
}
```

### Checking the scaling up of the pods.

```shell

$ kubectl -n artemis get pods
NAME                                READY   STATUS              RESTARTS   AGE
artemis-activemq-66c66ffdcc-9f7hq   2/2     Running             0          15m
artemis-consumer-589c9b87f7-mldrx   0/1     ContainerCreating   0          4s
artemis-consumer-589c9b87f7-mltqx   1/1     Running             0          14s
```

### Scale to zero

Once you stop the producer program, KEDA will determine that messages are no longer coming and will scale down the pods to zero.

```shell
$kubectl -n artemis get pods -w
NAME                                READY   STATUS    RESTARTS   AGE
artemis-activemq-66c66ffdcc-9f7hq   2/2     Running   0          16m
artemis-consumer-589c9b87f7-8xwq6   1/1     Running   0          43s
artemis-consumer-589c9b87f7-k2bf5   1/1     Running   0          12s
artemis-consumer-589c9b87f7-mldrx   1/1     Running   0          58s
artemis-consumer-589c9b87f7-mltqx   1/1     Running   0          68s
artemis-consumer-589c9b87f7-mltqx   1/1     Terminating   0          81s
artemis-consumer-589c9b87f7-8xwq6   1/1     Terminating   0          56s
artemis-consumer-589c9b87f7-mldrx   1/1     Terminating   0          71s
artemis-consumer-589c9b87f7-k2bf5   1/1     Terminating   0          25s
artemis-consumer-589c9b87f7-8xwq6   0/1     Terminating   0          57s
artemis-consumer-589c9b87f7-mldrx   0/1     Terminating   0          72s
artemis-consumer-589c9b87f7-k2bf5   0/1     Terminating   0          27s
artemis-consumer-589c9b87f7-mldrx   0/1     Terminating   0          73s
artemis-consumer-589c9b87f7-mldrx   0/1     Terminating   0          73s
artemis-consumer-589c9b87f7-mltqx   0/1     Terminating   0          83s

```

### Clean up

Delete the consumer

`skaffold delete -p consumer `

Delete ActiveMQ Artemis and the metrics provider

`skaffold delete -p metrics-provider`

Delete the External Scaler object

`kubectl delete -f k8s-manifest/externalscaler_scaledobject.yaml`

Verify that the HPA is successfully deleted

`kubectl -n artemis get hpa`

## License
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Fbalchua%2Fartemis-ext-scaler.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Fbalchua%2Fartemis-ext-scaler?ref=badge_large)
