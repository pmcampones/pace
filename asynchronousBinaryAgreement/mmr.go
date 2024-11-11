package asynchronousBinaryAgreement

import (
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"pace/utils"
	"sync/atomic"
)

var abaLogger = utils.GetLogger(slog.LevelWarn)

const firstRound = 0

type roundMsg struct {
	val byte
	r   uint16
}

type mmr struct {
	handler    *mmrHandler
	termGadget *mmrTermination
	commands   chan func()
	closeChan  chan struct{}
	isClosed   atomic.Bool
}

func newMMR(n, f uint, deliverBVal, deliverAux chan roundMsg, deliverDecision chan byte, coinReq chan uint16) *mmr {
	m := &mmr{
		handler:    newMMRHandler(n, f, deliverBVal, deliverAux, deliverDecision, coinReq),
		termGadget: newMmrTermination(f),
		commands:   make(chan func()),
		closeChan:  make(chan struct{}, 1),
		isClosed:   atomic.Bool{},
	}
	go m.invoker()
	return m
}

func (m *mmr) invoker() {
	for {
		select {
		case cmd := <-m.commands:
			cmd()
		case <-m.closeChan:
			abaLogger.Info("closing mmr")
			m.termGadget.close()
			m.handler.close()
			return
		}
	}
}

func (m *mmr) propose(est byte) error {
	if m.isClosed.Load() {
		abaLogger.Info("received proposal on closed mmr")
		return nil
	}
	abaLogger.Info("scheduling initial proposal estimate", "est", est)
	errChan := make(chan error)
	m.commands <- func() {
		errChan <- m.handler.propose(est, firstRound)
	}
	return <-errChan
}

func (m *mmr) submitBVal(bVal byte, sender uuid.UUID, r uint16) error {
	if m.isClosed.Load() {
		abaLogger.Info("received bVal on closed mmr")
		return nil
	}
	abaLogger.Debug("scheduling submit bVal", "bVal", bVal, "mmrRound", r, "sender", sender)
	errChan := make(chan error)
	m.commands <- func() {
		errChan <- m.handler.submitBVal(bVal, sender, r)
	}
	return <-errChan
}

func (m *mmr) submitAux(aux byte, sender uuid.UUID, r uint16) error {
	if m.isClosed.Load() {
		abaLogger.Info("received aux on closed mmr")
		return nil
	}
	abaLogger.Debug("scheduling submit aux", "aux", aux, "mmrRound", r)
	errChan := make(chan error)
	m.commands <- func() {
		errChan <- m.handler.submitAux(aux, sender, r)
	}
	return <-errChan
}

func (m *mmr) submitCoin(coin byte, r uint16) error {
	if m.isClosed.Load() {
		abaLogger.Info("received coin on closed mmr")
		return nil
	}
	abaLogger.Debug("scheduling submit coin", "coin", coin, "mmrRound", r)
	errChan := make(chan error)
	m.commands <- func() {
		errChan <- m.handler.submitCoin(coin, r)
	}
	return <-errChan
}

func (m *mmr) submitDecision(decision byte, sender uuid.UUID) (byte, error) {
	if m.isClosed.Load() {
		abaLogger.Info("received decision on closed mmr")
		return bot, nil
	}
	abaLogger.Debug("submitting decision", "decision", decision, "sender", sender)
	res := make(chan termOutput, 1)
	m.commands <- func() {
		result, err := m.termGadget.submitDecision(decision, sender)
		res <- termOutput{decision: result, err: err}
	}
	output := <-res
	if output.err != nil {
		return bot, fmt.Errorf("unable to submit decision: %v", output.err)
	}
	return output.decision, nil
}

func (m *mmr) close() {
	m.isClosed.Store(true)
	abaLogger.Info("signaling close mmr")
	m.closeChan <- struct{}{}
}

type cancelableRound struct {
	round     *mmrRound
	closeChan chan struct{}
}

func (r *cancelableRound) close() {
	r.closeChan <- struct{}{}
}

type mmrHandler struct {
	n               uint
	f               uint
	deliverBVal     chan roundMsg
	deliverAux      chan roundMsg
	deliverDecision chan byte
	hasDecided      bool
	coinReq         chan uint16
	rounds          map[uint16]*cancelableRound
	termGadget      *mmrTermination
}

func newMMRHandler(n, f uint, deliverBVal, deliverAux chan roundMsg, deliverDecision chan byte, coinReq chan uint16) *mmrHandler {
	return &mmrHandler{
		n:               n,
		f:               f,
		deliverBVal:     deliverBVal,
		deliverAux:      deliverAux,
		deliverDecision: deliverDecision,
		hasDecided:      false,
		coinReq:         coinReq,
		rounds:          make(map[uint16]*cancelableRound),
	}
}

func (m *mmrHandler) propose(est byte, r uint16) error {
	abaLogger.Debug("proposing estimate", "est", est, "mmrRound", r)
	if round, err := m.getRound(r); err != nil {
		return fmt.Errorf("unable to get round %d: %v", r, err)
	} else if err := round.round.propose(est); err != nil {
		return fmt.Errorf("unable to propose to round %d: %v", r, err)
	}
	return nil
}

func (m *mmrHandler) submitBVal(bVal byte, sender uuid.UUID, r uint16) error {
	if round, err := m.getRound(r); err != nil {
		return fmt.Errorf("unable to get round %d: %v", r, err)
	} else if err := round.round.submitBVal(bVal, sender); err != nil {
		return fmt.Errorf("unable to submit bVal to round %d: %v", r, err)
	}
	return nil
}

func (m *mmrHandler) submitAux(aux byte, sender uuid.UUID, r uint16) error {
	if round, err := m.getRound(r); err != nil {
		return fmt.Errorf("unable to get round %d: %v", r, err)
	} else if err := round.round.submitAux(aux, sender); err != nil {
		return fmt.Errorf("unable to submit aux to round %d: %v", r, err)
	}
	return nil
}

func (m *mmrHandler) submitCoin(coin byte, r uint16) error {
	if round, err := m.getRound(r); err != nil {
		return fmt.Errorf("unable to get round %d: %v", r, err)
	} else if res := round.round.submitCoin(coin); res.err != nil {
		return fmt.Errorf("unable to submit coin to round %d: %v", r, res.err)
	} else if res.decided && !m.hasDecided {
		m.hasDecided = true
		m.deliverDecision <- res.estimate
	} else if err := m.propose(res.estimate, r+1); err != nil {
		return fmt.Errorf("unable to propose to round %d: %v", r+1, err)
	}
	return nil
}

func (m *mmrHandler) getRound(rNum uint16) (*cancelableRound, error) {
	round := m.rounds[rNum]
	if round == nil {
		if r, err := m.newRound(rNum); err != nil {
			return nil, fmt.Errorf("unable to create new round: %v", err)
		} else {
			round = r
			m.rounds[rNum] = r
		}
	}
	return round, nil
}

func (m *mmrHandler) newRound(r uint16) (*cancelableRound, error) {
	round := newMMRRound(m.n, m.f)
	closeChan := make(chan struct{}, 1)
	go m.listenRequests(round, closeChan, r)
	return &cancelableRound{
		round:     round,
		closeChan: closeChan,
	}, nil
}

func (m *mmrHandler) listenRequests(round *mmrRound, close chan struct{}, rnum uint16) {
	for {
		select {
		case bVal := <-round.bValChan:
			abaLogger.Debug("received bVal", "bVal", bVal, "mmrRound", rnum)
			go func() {
				m.deliverBVal <- roundMsg{val: bVal, r: rnum}
			}()
		case aux := <-round.auxChan:
			abaLogger.Debug("received aux", "aux", aux, "mmrRound", rnum)
			go func() {
				m.deliverAux <- roundMsg{val: aux, r: rnum}
			}()
		case <-round.coinReqChan:
			abaLogger.Debug("received coin request", "mmrRound", rnum)
			go func() {
				m.coinReq <- rnum
			}()
		case <-close:
			abaLogger.Info("closing mmr round", "mmrRound", rnum)
			return
		}
	}
}

func (m *mmrHandler) close() {
	for _, round := range m.rounds {
		round.close()
	}
}
