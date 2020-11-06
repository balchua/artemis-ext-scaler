package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	pb "github.com/balchua/artemis-ext-scaler/externalscaler/proto"
	"github.com/golang/protobuf/ptypes/empty"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	port              int
	artemisURL        string
	artemisBrokerName string
	artemisAddress    string
	artemisQueueName  string
	userName          string
	password          string
	metricName        string
	targetSize        int
)

type monitoring struct {
	Request   string `json:"request"`
	MsgCount  int64  `json:"value"`
	Status    int    `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

type requestInfo struct {
	Mbean     string `json:"mbean"`
	Attribute string `json:"attribute"`
	Type      string `json:"type"`
}

type externalScalerServer struct {
	scaledObjectRef map[string][]*pb.ScaledObjectRef
}

func init() {

	// Only log the warning severity or above.
	log.SetLevel(log.InfoLevel)
}

func getArtemisEndpoint() string {
	return artemisURL + "/console/jolokia/read/org.apache.activemq.artemis" + ":broker=" + "\"" +
		artemisBrokerName + "\"" + ",component=addresses,address=" + "\"" +
		artemisAddress + "\"" + ",subcomponent=queues,routing-type=\"anycast\",queue=\"" +
		artemisQueueName + "\"" + "/MessageCount"
}

func (s *externalScalerServer) setBrokerDetails(scaledObject *pb.ScaledObjectRef) (*empty.Empty, error) {
	log.Infof("broker details is called.")
	out := new(empty.Empty)

	size, err := strconv.Atoi(scaledObject.ScalerMetadata["queueLength"])

	if err != nil {
		targetSize = 10
	} else {
		targetSize = size
	}

	artemisAddress = scaledObject.ScalerMetadata["brokerAddress"]
	artemisQueueName = scaledObject.ScalerMetadata["queueName"]
	metricName = artemisBrokerName + "-" + artemisAddress + "-" + artemisQueueName

	log.Infof("BrokerAddress: %s, Metrics Name %s, Queue Name: %s", artemisAddress, metricName, artemisQueueName)
	return out, nil
}

// Close
func (s *externalScalerServer) Close(ctx context.Context, scaledObjectRef *pb.ScaledObjectRef) (*empty.Empty, error) {
	out := new(empty.Empty)

	return out, nil
}

// IsActive
func (s *externalScalerServer) IsActive(ctx context.Context, in *pb.ScaledObjectRef) (*pb.IsActiveResponse, error) {
	out := new(pb.IsActiveResponse)
	out.Result = (s.getMessageCount() > 0)
	return out, nil
}

func (s *externalScalerServer) GetMetricSpec(ctx context.Context, in *pb.ScaledObjectRef) (*pb.GetMetricSpecResponse, error) {
	log.Info("Getting Metric Spec...")
	s.setBrokerDetails(in)
	out := new(pb.GetMetricSpecResponse)

	m := new(pb.MetricSpec)
	m.MetricName = metricName
	m.TargetSize = int64(targetSize)

	out.MetricSpecs = make([]*pb.MetricSpec, 1)

	out.MetricSpecs[0] = m
	log.Infof("Metrics Name: %s \n Metrics TargetSize: %d\n", out.MetricSpecs[0].MetricName, out.MetricSpecs[0].TargetSize)

	return out, nil
}

func (s *externalScalerServer) getMessageCount() int64 {
	var messageCount int64
	var monitoringInfo *monitoring
	messageCount = 0

	log.Info("getting the message count..")

	client := &http.Client{
		Timeout: time.Second * 3,
	}
	url := getArtemisEndpoint()
	log.Infof("URL: %s", url)
	req, err := http.NewRequest("GET", url, nil)

	req.SetBasicAuth(userName, password)
	req.Header.Add("Origin", "localhost")

	if err != nil {
		log.Errorf("Error while accessing ActiveMQ: %s", err)
		return 0
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error while accessing ActiveMQ: %s", err)
		return 0
	}

	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&monitoringInfo)
	if resp.StatusCode == 200 && monitoringInfo.Status == 200 {
		messageCount = monitoringInfo.MsgCount
	} else {
		log.Infof("Response Status %d", resp.StatusCode)
	}
	log.Infof("Request: %s", monitoringInfo.Request)
	log.Infof("Total messages: %d", messageCount)
	return messageCount
}

func (s *externalScalerServer) StreamIsActive(in *pb.ScaledObjectRef, stream pb.ExternalScaler_StreamIsActiveServer) error {
	log.Infof("Stream is active is called.")

	return nil
}

func (s *externalScalerServer) GetMetrics(ctx context.Context, in *pb.GetMetricsRequest) (*pb.GetMetricsResponse, error) {
	messageCount := s.getMessageCount()

	m := new(pb.MetricValue)
	m.MetricName = metricName
	m.MetricValue = messageCount

	out := new(pb.GetMetricsResponse)

	out.MetricValues = make([]*pb.MetricValue, 1)
	out.MetricValues[0] = m

	return out, nil
}

func newServer() *externalScalerServer {
	s := &externalScalerServer{}

	return s
}

func main() {
	flag.IntVar(&port, "port", 10000, "The server port")
	flag.StringVar(&artemisURL, "url", "http://localhost:8161", "The artemis server url")
	flag.StringVar(&artemisBrokerName, "broker", "artemis-activemq", "The artemis broker name")
	flag.StringVar(&userName, "user", "artemis", "The artemis broker address")
	flag.StringVar(&password, "password", "artemis", "The artemis broker address")

	flag.Parse()
	if password == "" {
		log.Fatalf("Invalid credential.")
		os.Exit(1)
	}
	fmt.Printf("Port: %d\n", port)
	fmt.Printf("URL: %s\n", artemisURL)
	fmt.Printf("User %s\n", userName)

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", 5050))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	reflection.Register(grpcServer)
	pb.RegisterExternalScalerServer(grpcServer, newServer())
	grpcServer.Serve(lis)
}
