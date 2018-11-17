package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	cid "github.com/ipfs/go-cid"
	iaddr "github.com/ipfs/go-ipfs-addr"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	inet "github.com/libp2p/go-libp2p-net"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	protocol "github.com/libp2p/go-libp2p-protocol"
	mh "github.com/multiformats/go-multihash"
)

// IPFS bootstrap nodes. Used to find other peers in the network.
var bootstrapPeers = []string{
	"/ip4/104.131.131.82/tcp/4001/ipfs/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	"/ip4/104.236.179.241/tcp/4001/ipfs/QmSoLPppuBtQSGwKDZT2M73ULpjvfd3aZ6ha4oFGL1KrGM",
	"/ip4/104.236.76.40/tcp/4001/ipfs/QmSoLV4Bbm51jM9C4gDYZQ9Cy3U6aXMJDAbzgu2fzaDs64",
	"/ip4/128.199.219.111/tcp/4001/ipfs/QmSoLSafTMBsPKadTEgaXctDQVcqN88CNLHXMkTNwMKPnu",
	"/ip4/178.62.158.247/tcp/4001/ipfs/QmSoLer265NRgSp2LA3dPaeykiS1J6DifTC88f5uVQKNAd",
}

var rendezvous = "meet me here"
var pingURL = protocol.ID("/v1/ping")

func handleStream(stream inet.Stream) {
	fmt.Println("Got a new stream!")

	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	readData(rw)
	writeData(rw, "pong")

	// 'stream' will stay open until you close it (or the other side closes it).
}
func readData(rw *bufio.ReadWriter) {
	str, err := rw.ReadString('\n')
	if err != nil {
		fmt.Printf("read data error: %v", err)
		return
	}

	fmt.Printf("received - %s", str)
}

func writeData(rw *bufio.ReadWriter, data string) {
	fmt.Printf("send - %s\n", data)
	if _, err := rw.WriteString(fmt.Sprintf("%s\n", data)); err != nil {
		fmt.Printf("send data error: %v", err)
	}
	rw.Flush()
}

func main() {
	help := flag.Bool("h", false, "Display Help")
	rendezvousString := flag.String("r", rendezvous, "Unique string to identify group of nodes. Share this with your friends to let them connect with you")
	listen := flag.Bool("l", false, "Work as server")
	flag.Parse()

	if *help {
		fmt.Printf("This program demonstrates a simple ping using libp2p\n\n")
		fmt.Printf("Usage: Run './ping -l' as listener mode, which reply pong; another terminal run './ping' to send ping to peer and receiver reply\n")

		os.Exit(0)
	}

	ctx := context.Background()

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	host, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}

	// Set a function as stream handler.
	// This function is called when a peer initiate a connection and starts a stream with this peer.
	host.SetStreamHandler(pingURL, handleStream)

	kadDht, err := dht.New(ctx, host)
	if err != nil {
		panic(err)
	}

	// Let's connect to the bootstrap nodes first. They will tell us about the other nodes in the network.
	for _, peerAddr := range bootstrapPeers {
		addr, _ := iaddr.ParseString(peerAddr)
		peerinfo, _ := pstore.InfoFromP2pAddr(addr.Multiaddr())

		if err := host.Connect(ctx, *peerinfo); err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Connection established with bootstrap node: ", *peerinfo)
		}
	}

	// We use a rendezvous point "meet me here" to announce our location.
	// This is like telling your friends to meet you at the Eiffel Tower.
	v1b := cid.V1Builder{Codec: cid.Raw, MhType: mh.SHA2_256}
	rendezvousPoint, _ := v1b.Sum([]byte(*rendezvousString))

	fmt.Printf("announcing ourselves...: %v\n", host.ID())
	tctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	if err := kadDht.Provide(tctx, rendezvousPoint, true); err != nil {
		panic(err)
	}

	if *listen {
		select {}
	}

	// Now, look for others who have announced
	// This is like your friend telling you the location to meet you.
	// 'FindProviders' will return 'PeerInfo' of all the peers which
	// have 'Provide' or announced themselves previously.
	found := false
	for {
		time.Sleep(time.Second)
		if found {
			break
		}

		fmt.Println("searching for other peers...")
		tctx, cancel = context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		peers, err := kadDht.FindProviders(tctx, rendezvousPoint)
		if err != nil {
			fmt.Printf("find providers error: %v", err)
			continue
		}
		for _, p := range peers {
			if p.ID == host.ID() || len(p.Addrs) == 0 {
				// No sense connecting to ourselves or if addrs are not available
				continue
			}

			stream, err := host.NewStream(ctx, p.ID, pingURL)
			if err != nil {
				fmt.Printf("new stream: %v", err)
				continue
			}
			rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

			go func() {
				for {
					writeData(rw, "ping")
					readData(rw)
					time.Sleep(time.Second)
				}
			}()

			fmt.Println("Connected to: ", p)
			found = true
		}
	}

	select {}

}
