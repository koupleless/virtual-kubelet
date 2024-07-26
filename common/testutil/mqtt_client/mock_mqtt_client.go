package mqtt_client

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"strings"
	"sync"
	"time"
)

var _ mqtt.Client = &MockMqttClient{}

var _ mqtt.Token = &MockToken{}

var _ mqtt.Message = &MockMessage{}

var msgQueue map[string]chan mqtt.Message

func init() {
	msgQueue = make(map[string]chan mqtt.Message)
}

type MockMqttClient struct {
	sync.Mutex
	opts    *mqtt.ClientOptions
	subMap  map[string]mqtt.MessageHandler
	msgChan chan mqtt.Message
}

type MockToken struct {
	done     chan struct{}
	client   mqtt.Client
	funcCall func(client mqtt.Client)
}

type MockMessage struct {
	T string
	P []byte
}

func (m *MockMessage) Duplicate() bool {
	panic("implement me")
}

func (m *MockMessage) Qos() byte {
	panic("implement me")
}

func (m *MockMessage) Retained() bool {
	panic("implement me")
}

func (m *MockMessage) Topic() string {
	return m.T
}

func (m *MockMessage) MessageID() uint16 {
	panic("implement me")
}

func (m *MockMessage) Payload() []byte {
	return m.P
}

func (m *MockMessage) Ack() {}

func (m *MockToken) Wait() bool {
	m.funcCall(m.client)
	close(m.done)
	return true
}

func (m *MockToken) WaitTimeout(_ time.Duration) bool {
	m.funcCall(m.client)
	close(m.done)
	return true
}

func (m *MockToken) Done() <-chan struct{} {
	return m.done
}

func (m *MockToken) Error() error {
	return nil
}

func NewMockMqttClient(opt *mqtt.ClientOptions) mqtt.Client {
	msgChan := make(chan mqtt.Message, 1000)
	clientID := uuid.New().String()
	opt.SetClientID(clientID)
	return &MockMqttClient{
		opts:    opt,
		subMap:  make(map[string]mqtt.MessageHandler),
		msgChan: msgChan,
	}
}

func (m *MockMqttClient) IsConnected() bool {
	return true
}

func (m *MockMqttClient) IsConnectionOpen() bool {
	return true
}

func (m *MockMqttClient) Connect() mqtt.Token {
	msgQueue[m.opts.ClientID] = m.msgChan
	go func() {
		msg := <-m.msgChan
		// check subscription match
		for subTopic, handler := range m.subMap {
			if topicMatched(msg.Topic(), subTopic) {
				handler(m, msg)
			}
		}
	}()
	return &MockToken{done: make(chan struct{}), client: m, funcCall: m.opts.OnConnect}
}

func (m *MockMqttClient) Disconnect(_ uint) {
	m.Lock()
	defer m.Unlock()
	m.opts.OnConnectionLost(m, nil)
	m.subMap = make(map[string]mqtt.MessageHandler)
	delete(msgQueue, m.opts.ClientID)
}

func (m *MockMqttClient) Publish(topic string, _ byte, _ bool, payload interface{}) mqtt.Token {
	m.Lock()
	defer m.Unlock()
	message := MockMessage{
		T: topic,
		P: payload.([]byte),
	}
	for _, q := range msgQueue {
		// send to all client
		q <- &message
	}
	return &MockToken{done: make(chan struct{}), client: m, funcCall: func(client mqtt.Client) {}}
}

func (m *MockMqttClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	m.Lock()
	defer m.Unlock()
	m.subMap[topic] = callback
	return &MockToken{done: make(chan struct{}), client: m, funcCall: func(client mqtt.Client) {}}
}

func (m *MockMqttClient) SubscribeMultiple(filters map[string]byte, callback mqtt.MessageHandler) mqtt.Token {
	panic("implement me")
}

func (m *MockMqttClient) Unsubscribe(topics ...string) mqtt.Token {
	m.Lock()
	defer m.Unlock()
	for _, topic := range topics {
		delete(m.subMap, topic)
	}
	return &MockToken{done: make(chan struct{}), client: m, funcCall: func(client mqtt.Client) {}}
}

func (m *MockMqttClient) AddRoute(topic string, callback mqtt.MessageHandler) {
	panic("implement me")
}

func (m *MockMqttClient) OptionsReader() mqtt.ClientOptionsReader {
	return mqtt.ClientOptionsReader{}
}

func topicMatched(pubTopic, subTopic string) bool {
	pubTopicList := strings.Split(pubTopic, "/")
	subTopicList := strings.Split(subTopic, "/")
	return checkTopicMatched(pubTopicList, subTopicList)
}

func checkTopicMatched(pubTopicList, subTopicList []string) bool {
	if len(pubTopicList) == 0 && len(subTopicList) == 0 {
		return true
	}
	if len(pubTopicList) == 0 || len(subTopicList) == 0 {
		return false
	}
	if subTopicList[0] == "+" {
		return checkTopicMatched(pubTopicList[1:], subTopicList[1:])
	}
	if subTopicList[0] == "*" {
		return checkTopicMatched(pubTopicList[1:], subTopicList[1:]) || checkTopicMatched(pubTopicList[1:], subTopicList)
	}
	if subTopicList[0] == pubTopicList[0] {
		return checkTopicMatched(pubTopicList[1:], subTopicList[1:])
	}
	return false
}
