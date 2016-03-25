package mqtt

import (
	"bytes"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/disorganizer/brig/id"
	"github.com/disorganizer/brig/transfer"
	"github.com/disorganizer/brig/transfer/wire"
	"github.com/gogo/protobuf/proto"
	"github.com/surgemq/message"
	"github.com/surgemq/surgemq/service"
)

type client struct {
	layer        *Layer
	client       *service.Client
	peer         id.Peer
	execRequests bool

	// A unique client id, used for the mqtt id.
	clientIdx uint32

	// Last time we heard from our peer
	// (not only for ping, but for all operations)
	lastHearbeat time.Time
}

var GlobalClientIdx = uint32(0)

func newClient(lay *Layer, peer id.Peer, execRequests bool) (*client, error) {
	defer atomic.AddUint32(&GlobalClientIdx, 1)

	return &client{
		layer:        lay,
		client:       nil,
		execRequests: execRequests,
		peer:         peer,
		lastHearbeat: time.Now(),
		clientIdx:    GlobalClientIdx,
	}, nil
}

func (cv *client) peerTopic(sub string) []byte {
	return []byte(fmt.Sprintf("%s/%s", cv.peer.Hash(), sub))
}

func (cv *client) formatClientID() []byte {
	return []byte(fmt.Sprintf("%s%d", cv.peer.Hash(), cv.clientIdx))
}

func (cv *client) heartbeat(msg, ack message.Message, err error) error {
	if err != nil {
		return err
	}

	// BEAT IT, JUST BEAT IT!
	cv.lastHearbeat = time.Now()
	return nil
}

func (cv *client) publish(data []byte, topic []byte) error {
	pubmsg := message.NewPublishMessage()
	pubmsg.SetTopic(topic)
	pubmsg.SetPayload(data)
	pubmsg.SetQoS(2)

	return cv.client.Publish(pubmsg, cv.heartbeat)
}

func (cv *client) notifyStatus(status string) error {
	return cv.publish(
		[]byte(status),
		cv.peerTopic("status/"+cv.layer.self.Hash()),
	)
}

func (cv *client) processRequest(msg *message.PublishMessage, answer bool) error {
	if !cv.execRequests {
		return nil
	}

	parts := bytes.SplitN(msg.Topic(), []byte{'/'}, 3)
	if len(parts) < 3 {
		return fmt.Errorf("Bad topic: %v", msg.Topic())
	}

	reqData := msg.Payload()
	req := &wire.Request{}

	if err := proto.Unmarshal(reqData, req); err != nil {
		return err
	}

	handler, ok := cv.layer.handlers[req.GetReqType()]
	if !ok {
		return fmt.Errorf("No such request handler: %d", req.GetReqType())
	}

	resp, err := handler(cv.layer, req)
	if err != nil {
		return err
	}

	if !answer || (resp == nil && err == nil) {
		return nil
	}

	// Respond error back if any:
	if resp == nil {
		resp = &wire.Response{
			Error: proto.String(err.Error()),
		}
	}

	// Autofill the internal fields:
	resp.ID = proto.Int64(req.GetID())
	resp.ReqType = req.GetReqType().Enum()

	respData, err := proto.Marshal(resp)
	if err != nil {
		fmt.Println("proto failed")
		log.Debugf("Invalid proto response: %v", err)
		return err
	}

	respTopic := fmt.Sprintf(
		"%s/response/%s",
		parts[2],
		cv.layer.self.Hash(),
	)

	fmt.Println("Publish response back to", respTopic)

	// Publish response:
	if err := cv.publish(respData, []byte(respTopic)); err != nil {
		return err
	}

	return nil
}

func (cv *client) handleStatus(msg *message.PublishMessage) error {
	data := msg.Payload()

	parsed := bytes.SplitN(msg.Topic(), []byte{'/'}, 3)
	if len(parsed) != 3 {
		return fmt.Errorf("Invalid online notification: %s", msg.Topic())
	}

	// TODO: Somehow update Layer's online infos.
	fmt.Printf("# %s is going %s\n", parsed[2], string(data))
	return nil
}

func (cv *client) handleRequests(msg *message.PublishMessage) error {
	return cv.processRequest(msg, true)
}

func (cv *client) handleBroadcast(msg *message.PublishMessage) error {
	return cv.processRequest(msg, false)
}

func (cv *client) handleResponse(msg *message.PublishMessage) error {
	resp := &wire.Response{}
	if err := proto.Unmarshal(msg.Payload(), resp); err != nil {
		return err
	}

	// Send the response to the requesting client:
	fmt.Println("Handle response", resp)
	if err := cv.layer.forwardResponse(resp); err != nil {
		log.Warningf("forward failed: %v", err)
	}

	return nil
}

func (cv *client) connect(addr net.Addr) error {
	msg := message.NewConnectMessage()
	msg.SetVersion(4)
	msg.SetCleanSession(true)
	msg.SetClientId(cv.formatClientID())
	msg.SetKeepAlive(300)
	msg.SetWillQos(1)

	// Where to publish our death:
	msg.SetWillTopic(cv.peerTopic("status"))
	msg.SetWillMessage([]byte(
		fmt.Sprintf(
			"%s-%s",
			cv.layer.self.Hash(),
			"offline",
		),
	))

	client := &service.Client{}

	fullAddr := "tcp://" + addr.String()
	if err := client.Connect(fullAddr, msg); err != nil {
		return err
	}

	topicHandlers := map[string]func(msg *message.PublishMessage) error{
		"broadcast":  cv.handleBroadcast,
		"response/+": cv.handleResponse,
	}

	if cv.execRequests {
		topicHandlers["status/+"] = cv.handleStatus
		topicHandlers["request/+"] = cv.handleRequests
	}

	fmt.Println("Im ", cv.layer.self.Hash())
	for name, handler := range topicHandlers {
		submsg := message.NewSubscribeMessage()
		submsg.AddTopic(cv.peerTopic(name), 2)
		fmt.Println("  Subscribing to", string(cv.peerTopic(name)))

		// There does not seem to be an easier way to register
		// different callbacks per
		if err := client.Subscribe(submsg, cv.heartbeat, handler); err != nil {
			return err
		}
	}

	cv.client = client

	if err := cv.notifyStatus("online"); err != nil {
		log.Warningf("Could not publish an online notify: %v", err)
	}

	return nil
}

func (cv *client) disconnect() error {
	if cv.client != nil {
		return nil
	}

	if err := cv.notifyStatus("offline"); err != nil {
		log.Warningf("Could not publish an offline notify: %v", err)
	}

	cv.client.Disconnect()
	cv.client = nil
	return nil
}

func (cv *client) SendAsync(req *wire.Request, handler transfer.AsyncFunc) error {
	respnotify := cv.layer.addReqRespPair(req)

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	// Start before publish to fix a very unlikely race.
	go func() {
		// Guard with a timeout to protect against too many go routines.
		ticker := time.NewTicker(30 * time.Second)

		select {
		case resp, ok := <-respnotify:
			if resp != nil && ok && handler != nil {
				handler(resp)
			}
		case <-ticker.C:
		}
	}()

	reqTopic := cv.peerTopic("request/" + cv.layer.self.Hash())
	log.Debugf("Publish request on %s", reqTopic)
	if err := cv.publish(data, reqTopic); err != nil {
		return err
	}

	return nil
}

func (cv *client) Close() error {
	return cv.disconnect()
}

func (cv *client) ping() (bool, error) {
	if cv.client == nil {
		return false, transfer.ErrOffline
	}

	if time.Since(cv.lastHearbeat) < 10*time.Second {
		return true, nil
	}

	// Ping() seems to wait for the ACK.
	if err := cv.client.Ping(cv.heartbeat); err != nil {
		return false, err
	}

	return true, nil
}

func (cv *client) Peer() id.Peer {
	return cv.peer
}