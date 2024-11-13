package agreementCommonSubset

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	aba "pace/asynchronousBinaryAgreement"
	brb "pace/byzantineReliableBroadcast"
	ct "pace/coinTosser"
	on "pace/overlayNetwork"
	"slices"
	"testing"
)

func TestShouldOutputOwnProposal2(t *testing.T) {
	testShouldOutputProposalsAlt(t, 1, 0)
}

func TestShouldOutputProposalsNoFaults2(t *testing.T) {
	testShouldOutputProposalsAlt(t, 10, 0)
}

func TestShouldOutputProposalsMaxFaults2(t *testing.T) {
	f := uint(3)
	n := 3*f + 1
	testShouldOutputProposalsAlt(t, n, f)
}

func testShouldOutputProposalsAlt(t *testing.T, n, f uint) {
	nodes := lo.Map(lo.Range(int(n)), func(i int, _ int) *on.Node {
		address := fmt.Sprintf("localhost:%d", 6000+i)
		return on.GetNode(t, address, "localhost:6000")
	})
	abachans := getAbachans(t, n, f, nodes)
	id := uuid.New()
	proposers := lo.Map(nodes, func(node *on.Node, _ int) uuid.UUID { return uuid.New() })
	bkr2Instances := lo.Map(abachans, func(abachan *aba.AbaChannel, _ int) *BKR {
		return NewBKR(id, f, proposers, abachan)
	})
	for _, bkr := range bkr2Instances {
		for i, participant := range proposers {
			input := []byte(fmt.Sprintf("input%d", i))
			assert.NoError(t, bkr.receiveInput(input, participant))
		}
	}
	outputs := lo.Map(bkr2Instances, func(bkr *BKR, _ int) [][]byte { return <-bkr.output })
	assert.True(t, uint(len(outputs[0])) >= f)
	firstOutput := outputs[0]
	assert.True(t, lo.EveryBy(outputs, func(output [][]byte) bool { return equalsOutputs(output, firstOutput) }))
	assert.True(t, lo.EveryBy(nodes, func(node *on.Node) bool { return node.Disconnect() == nil }))
}

func getAbachans(t *testing.T, n uint, f uint, nodes []*on.Node) []*aba.AbaChannel {
	dealSSs := lo.Map(nodes, func(node *on.Node, _ int) *on.SSChannel { return on.CreateSSChannel(node, 'd') })
	ctBebs := lo.Map(nodes, func(node *on.Node, _ int) *on.BEBChannel { return on.CreateBEBChannel(node, 'c') })
	mBebs := lo.Map(nodes, func(node *on.Node, _ int) *on.BEBChannel { return on.CreateBEBChannel(node, 'm') })
	tBebs := lo.Map(nodes, func(node *on.Node, _ int) *on.BEBChannel { return on.CreateBEBChannel(node, 't') })
	tBrbs := lo.Map(tBebs, func(beb *on.BEBChannel, _ int) *brb.BRBChannel { return brb.CreateBRBChannel(n, f, beb) })
	on.InitializeNodes(t, nodes)
	assert.NoError(t, ct.DealSecret(dealSSs[0], ct.NewScalar(42), f))
	abachans := lo.ZipBy4(dealSSs, ctBebs, mBebs, tBrbs, func(dealSS *on.SSChannel, ctBeb *on.BEBChannel, mBeb *on.BEBChannel, tBrb *brb.BRBChannel) *aba.AbaChannel {
		abachan, err := aba.NewAbaChannel(n, f, dealSS, ctBeb, mBeb, tBrb)
		assert.NoError(t, err)
		return abachan
	})
	return abachans
}

func equalsOutputs(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	return lo.EveryBy(lo.Zip2(a, b), func(tuple lo.Tuple2[[]byte, []byte]) bool {
		return slices.Equal(tuple.A, tuple.B)
	})
}
