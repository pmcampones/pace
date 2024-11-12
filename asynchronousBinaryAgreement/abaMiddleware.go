package asynchronousBinaryAgreement

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	on "pace/overlayNetwork"
	"pace/utils"
)

type middlewareCode byte

const (
	bval middlewareCode = 'a' + iota
	aux
)

type abaMsg struct {
	sender   uuid.UUID
	instance uuid.UUID
	kind     middlewareCode
	round    uint16
	val      byte
}

type abaMiddleware struct {
	beb       *on.BEBChannel
	output    chan *abaMsg
	closeChan chan struct{}
}

func newABAMiddleware(beb *on.BEBChannel) *abaMiddleware {
	m := &abaMiddleware{
		beb:       beb,
		output:    make(chan *abaMsg),
		closeChan: make(chan struct{}),
	}
	go m.bebDeliver()
	return m
}

func (m *abaMiddleware) bebDeliver() {
	for {
		select {
		case bebMsg := <-m.beb.GetBEBChan():
			m.processMsg(bebMsg)
		case <-m.closeChan:
			abaChannelLogger.Info("closing byzantineReliableBroadcast abaMiddleware")
			return
		}
	}
}

func (m *abaMiddleware) processMsg(bebMsg on.BEBMsg) {
	if amsg, err := m.parseMsg(bebMsg.Content, bebMsg.Sender); err != nil {
		abaChannelLogger.Warn("unable to processMsg message during beb delivery", "error", err)
	} else {
		abaChannelLogger.Debug("received message from beb", "sender", amsg.sender, "type", amsg.kind, "instance", amsg.instance, "val", amsg.val)
		go func() { m.output <- amsg }()
	}
}

func (m *abaMiddleware) parseMsg(msg []byte, sender *ecdsa.PublicKey) (*abaMsg, error) {
	tm := &abaMsg{}
	senderId, err := utils.PkToUUID(sender)
	if err != nil {
		return nil, fmt.Errorf("unable to convert sender public key to uuid: %v", err)
	}
	tm.sender = senderId
	reader := bytes.NewReader(msg)
	if id, err := utils.ExtractIdFromMessage(reader); err != nil {
		return nil, fmt.Errorf("unable to extract instance id from message: %v", err)
	} else {
		tm.instance = id
	}
	if kindByte, err := reader.ReadByte(); err != nil {
		return nil, fmt.Errorf("unable to read kind from message: %v", err)
	} else {
		tm.kind = middlewareCode(kindByte)
	}
	if err := binary.Read(reader, binary.LittleEndian, &tm.round); err != nil {
		return nil, fmt.Errorf("unable to read round from message: %v", err)
	} else if err := binary.Read(reader, binary.LittleEndian, &tm.val); err != nil {
		return nil, fmt.Errorf("unable to read val from message: %v", err)
	}
	return tm, nil
}

func (m *abaMiddleware) broadcastBVal(instance uuid.UUID, round uint16, val byte) error {
	return m.broadcastMsg(instance, bval, round, val)
}

func (m *abaMiddleware) broadcastAux(instance uuid.UUID, round uint16, val byte) error {
	return m.broadcastMsg(instance, aux, round, val)
}

func (m *abaMiddleware) broadcastMsg(instance uuid.UUID, kind middlewareCode, round uint16, val byte) error {
	buf := bytes.NewBuffer([]byte{})
	writer := bufio.NewWriter(buf)
	if instanceBytes, err := instance.MarshalBinary(); err != nil {
		return fmt.Errorf("unable to marshal instance id: %v", err)
	} else if n, err := writer.Write(instanceBytes); err != nil || n != len(instanceBytes) {
		return fmt.Errorf("unable to write instance id to buffer: %v", err)
	} else if err := writer.WriteByte(byte(kind)); err != nil {
		return fmt.Errorf("unable to write kind to buffer: %v", err)
	} else if err := binary.Write(writer, binary.LittleEndian, round); err != nil {
		return fmt.Errorf("unable to write round to buffer: %v", err)
	} else if err := writer.WriteByte(val); err != nil {
		return fmt.Errorf("unable to write val to buffer: %v", err)
	} else if err := writer.Flush(); err != nil {
		return fmt.Errorf("unable to flush writer: %v", err)
	} else if err := m.beb.BEBroadcast(buf.Bytes()); err != nil {
		return fmt.Errorf("unable to broadcast message: %v", err)
	}
	return nil
}

func (m *abaMiddleware) close() {
	abaChannelLogger.Info("sending close signal to abaMiddleware")
	m.closeChan <- struct{}{}
}
