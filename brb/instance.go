package brb

import (
	"fmt"
	. "github.com/google/uuid"
	"log/slog"
	"pace/utils"
)

var instanceLogger = utils.GetLogger(slog.LevelWarn)

type brbInstance struct {
	handler   *brbHandler
	commands  chan<- func()
	closeChan chan<- struct{}
}

func newBrbInstance(n, f uint, echo, ready, output chan []byte) *brbInstance {
	handler := newBrbHandler(n, f, echo, ready, output)
	commands := make(chan func())
	closeChan := make(chan struct{})
	executor := &brbInstance{
		handler:   handler,
		commands:  commands,
		closeChan: closeChan,
	}
	go executor.invoker(commands, closeChan)
	return executor
}

func (e *brbInstance) send(msg []byte) {
	instanceLogger.Debug("submitting send message handling command")
	e.commands <- func() {
		e.handler.handleSend(msg)
	}
}

func (e *brbInstance) echo(msg []byte, sender UUID) error {
	instanceLogger.Debug("submitting echo message handling command")
	errChan := make(chan error)
	e.commands <- func() {
		errChan <- e.handler.handleEcho(msg, sender)
	}
	return <-errChan
}

func (e *brbInstance) ready(msg []byte, sender UUID) error {
	instanceLogger.Debug("submitting ready message handling command")
	errChan := make(chan error)
	e.commands <- func() {
		errChan <- e.handler.handleReady(msg, sender)
	}
	return <-errChan
}

func (e *brbInstance) invoker(commands <-chan func(), closeChan <-chan struct{}) {
	for {
		select {
		case command := <-commands:
			command()
		case <-closeChan:
			instanceLogger.Info("closing executor")
			return
		}
	}
}

func (e *brbInstance) close() {
	instanceLogger.Info("sending signal to close bcb handler")
	e.closeChan <- struct{}{}
}

type brbHandler struct {
	data         *brbData
	peersEchoed  map[UUID]bool
	peersReadied map[UUID]bool
	handler      *brbPhase1Handler
}

type brbData struct {
	n       uint
	f       uint
	echoes  map[UUID]uint
	readies map[UUID]uint
}

func newBrbHandler(n, f uint, echo, ready, output chan []byte) *brbHandler {
	data := brbData{
		n:       n,
		f:       f,
		echoes:  make(map[UUID]uint),
		readies: make(map[UUID]uint),
	}
	ph3 := newPhase3Handler(&data, output)
	ph2 := newPhase2Handler(&data, ready, ph3)
	ph1 := newPhase1Handler(&data, echo, ph2)
	instance := &brbHandler{
		data:         &data,
		peersEchoed:  make(map[UUID]bool),
		peersReadied: make(map[UUID]bool),
		handler:      ph1,
	}
	return instance
}

func (h *brbHandler) handleSend(msg []byte) {
	h.handler.handleSend(msg)
}

func (h *brbHandler) handleEcho(msg []byte, sender UUID) error {
	instanceLogger.Debug("submitting echo message")
	ok := h.peersEchoed[sender]
	if ok {
		return fmt.Errorf("already received echo from peer %s", sender)
	}
	mid := utils.BytesToUUID(msg)
	h.data.echoes[mid]++
	h.peersEchoed[sender] = true
	err := h.handler.handleEcho(msg, mid)
	if err != nil {
		return fmt.Errorf("unable to handle echo: %v", err)
	}
	return nil
}

func (h *brbHandler) handleReady(msg []byte, sender UUID) error {
	instanceLogger.Debug("submitting ready message")
	ok := h.peersReadied[sender]
	if ok {
		return fmt.Errorf("already received ready from peer %s", sender)
	}
	mid := utils.BytesToUUID(msg)
	h.data.readies[mid]++
	h.peersReadied[sender] = true
	err := h.handler.handleReady(msg, mid)
	if err != nil {
		return fmt.Errorf("unable to handle ready: %v", err)
	}
	return nil
}
