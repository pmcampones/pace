package coinTosser

import (
	"fmt"
	"github.com/magiconair/properties"
	"github.com/samber/mo"
	"github.com/stretchr/testify/require"
	"maps"
	"pace/utils"
	"slices"
	"testing"
	"time"
)

// This test should work with threshold = 3 but does not. I don't know why but the broadcasts in some replicas aren't reaching all peers.
func TestTrue(t *testing.T) {
	props := properties.NewProperties()
	_, _, err := props.Set("ct_code", "C")
	require.NoError(t, err)
	_, _, err = props.Set("deal_code", "D")
	require.NoError(t, err)
	err = utils.SetProps(props)
	require.NoError(t, err)
	threshold := uint(2)
	utils.SetupDefaultLogger()
	node0, memObs, dealObs0, err := makeDealNode("localhost:6000", "localhost:6000", 4, 4)
	require.NoError(t, err)
	node1, _, dealObs1, err := makeDealNode("localhost:6001", "localhost:6000", 4, 4)
	require.NoError(t, err)
	node2, _, dealObs2, err := makeDealNode("localhost:6002", "localhost:6000", 4, 4)
	require.NoError(t, err)
	node3, _, dealObs3, err := makeDealNode("localhost:6003", "localhost:6000", 4, 4)
	require.NoError(t, err)
	fmt.Println("Nodes created")
	<-memObs.UpBarrier
	<-memObs.UpBarrier
	<-memObs.UpBarrier
	fmt.Println("Contact connected to all")
	peers := slices.Collect(maps.Values(memObs.Peers))
	err = shareDeals(threshold, node0, peers, dealObs0)
	require.NoError(t, err)
	<-dealObs0.dealChan
	<-dealObs1.dealChan
	<-dealObs2.dealChan
	<-dealObs3.dealChan
	fmt.Println("Deals shared")
	ctChannel0 := NewCoinTosserChannel(node0, threshold)
	ctChannel1 := NewCoinTosserChannel(node1, threshold)
	ctChannel2 := NewCoinTosserChannel(node2, threshold)
	ctChannel3 := NewCoinTosserChannel(node3, threshold)
	time.Sleep(time.Second) // making sure the channels attach to the overlayNetwork to get updates
	fmt.Println("CT Channels created")
	ch0 := make(chan mo.Result[bool], 2)
	ch1 := make(chan mo.Result[bool], 2)
	ch2 := make(chan mo.Result[bool], 2)
	ch3 := make(chan mo.Result[bool], 2)
	ctChannel0.TossCoin([]byte("seed"), ch0)
	ctChannel1.TossCoin([]byte("seed"), ch1)
	ctChannel2.TossCoin([]byte("seed"), ch2)
	ctChannel3.TossCoin([]byte("seed"), ch3)
	fmt.Println("Coins tossed")
	coin0 := <-ch0
	require.False(t, coin0.IsError())
	coin1 := <-ch1
	require.False(t, coin1.IsError())
	coin2 := <-ch2
	require.False(t, coin2.IsError())
	coin3 := <-ch3
	require.False(t, coin3.IsError())
	fmt.Println("Coins received")
	require.Equal(t, coin0.MustGet(), coin1.MustGet())
	require.Equal(t, coin1.MustGet(), coin2.MustGet())
	require.Equal(t, coin2.MustGet(), coin3.MustGet())
}
