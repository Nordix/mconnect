// Project page; https://github.com/Nordix/mconnect/
// LICENSE; MIT. See the "LICENSE" file in the Project page.
// Copyright (c) 2018, Nordix Foundation

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Nordix/mconnect/pkg/rndip"
)

var version = "unknown"

const helptext = `
Mconnect make many connects towards an address. The address is
supposed to be a virtual ip-address (vip) that is load-balanced to
many servers. Statistics per server-hostname is printed on exit. The
purpose is testing of connectivity and load balancing.

The "maxconcurrent" should in general be lower than the listen backlog
of the server. The go way of defining the backlog is to take the value
from the;

  /proc/sys/net/core/somaxconn

file which is the max value according to listen(2) (default 128).

Options;
`

type config struct {
	isServer      *bool
	addr          *string
	src           *string
	nconn         *int
	k8sprobe      *string
	keep          *bool
	udp           *bool
	version       *bool
	seed          *int
	maxconcurrent *int
	output        *string
	timeout       *time.Duration
	limiter       chan int
	wg            sync.WaitGroup
	rndip         *rndip.Rndip
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), helptext)
		flag.PrintDefaults()
	}

	var cmd config
	cmd.isServer = flag.Bool("server", false, "Act as server")
	cmd.addr = flag.String("address", "[::1]:5001", "Server address")
	cmd.src = flag.String("srccidr", "", "Base source CIDR to use")
	cmd.nconn = flag.Int("nconn", 1, "Number of connections")
	cmd.keep = flag.Bool("keep", false, "Keep connections open")
	cmd.k8sprobe = flag.String("k8sprobe", "", "k8s liveness address (http)")
	cmd.udp = flag.Bool("udp", false, "Use UDP")
	cmd.version = flag.Bool("version", false, "Print version and quit")
	cmd.seed = flag.Int("seed", 0, "Rnd seed. 0 = init from time")
	cmd.maxconcurrent = flag.Int("maxconcurrent", 64, "Max concurrent connects")
	cmd.output = flag.String("output", "txt", "Output format; json|txt")
	cmd.timeout = flag.Duration("timeout", 0, "Timeout")

	flag.Parse()
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(0)
	}

	if *cmd.version {
		fmt.Println(version)
		os.Exit(0)
	}

	if *cmd.isServer {
		os.Exit(cmd.server())
	} else {
		os.Exit(cmd.client())
	}
}

func (c *config) client() int {

	if *c.seed != 0 {
		rand.Seed(int64(*c.seed))
	} else {
		rand.Seed(time.Now().UnixNano())
	}

	c.limiter = make(chan int, *c.maxconcurrent)
	go statsWorker(&c.wg)

	if *c.src != "" {
		var err error
		if c.rndip, err = rndip.New(*c.src); err != nil {
			log.Fatal("scrcidr", err)
		}
	}

	// Connects have a default timeout of 2min. So if all connects
	// times out the execution time would be horrible for say 10000
	// connect attempts. So set a dead-line. We assume that 1000
	// connects/sec is supported and we allow a margin of 2 sec.
	if *c.keep {
		stats.Timeout = time.Hour
	} else {
		if *c.timeout != time.Duration(0) {
			stats.Timeout = *c.timeout
		} else {
			stats.Timeout = time.Duration(*c.nconn * int(time.Second) / 1000)
			stats.Timeout += 2 * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), stats.Timeout)
	defer cancel()

	stats.Start = time.Now()
	stats.Connects = *c.nconn
	c.wg.Add(*c.nconn)
	for i := 0; i < *c.nconn; i++ {
		if *c.udp {
			go c.udpConnect(ctx)
		} else {
			go c.connect(ctx)
		}
	}
	c.wg.Wait()

	c.wg.Add(1)
	hostch <- "QUIT!"
	c.wg.Wait()

	stats.Duration = time.Since(stats.Start)
	if *c.output == "json" {
		json.NewEncoder(os.Stdout).Encode(stats)
	} else {
		fmt.Println("Failed connects;", stats.FailedConnects)
		fmt.Println("Failed reads;", stats.FailedReads)
		for h, i := range stats.Hostmap {
			fmt.Println(h, i)
		}
	}
	if stats.FailedConnects > 0 || stats.FailedReads > 0 {
		return 1
	}
	return 0
}

type server struct {
	hostname string
}

func (c *config) server() int {

	// About listen backlog; /proc/sys/net/core/somaxconn and man listen(2)
	l, err := net.Listen("tcp", *c.addr)
	if err != nil {
		log.Fatal("listen", err)
		return -1
	}
	log.Println("Listen on address; ", *c.addr)
	obj := new(server)
	if obj.hostname, err = os.Hostname(); err != nil {
		log.Fatal("os.Hostname", err)
		return -1
	}

	if *c.udp {
		c.udpServer(obj.hostname)
	}

	for {
		if conn, err := l.Accept(); err != nil {
			log.Fatal(err)
		} else {
			go obj.handleRequest(conn)
		}
	}

	return 0
}

func (c *config) udpServer(hostname string) {

	pc, err := net.ListenPacket("udp", *c.addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listen on UDP address; ", *c.addr)

	rd := func(pc net.PacketConn) {
		buf := make([]byte, 9000)
		for {
			_, addr, err := pc.ReadFrom(buf)
			if err != nil {
				continue
			}
			pc.WriteTo([]byte(hostname), addr)
		}
	}
	for i := 0; i < runtime.NumCPU(); i++ {
		go rd(pc)
	}
}

// Handles incoming requests.
func (obj *server) handleRequest(conn net.Conn) {
	defer conn.Close()
	conn.Write([]byte(obj.hostname))
	buf := make([]byte, 1024)
	for {
		if _, err := conn.Read(buf); err != nil {
			return
		}
		conn.Write([]byte(obj.hostname))
	}
}

func (c *config) Help() string {
	return helptext
}

func (c *config) connect(ctx context.Context) {
	defer c.wg.Done()
	var d net.Dialer
	if c.rndip != nil {
		sadr := fmt.Sprintf("%s:0", c.rndip.GetIPString())
		//log.Println("Using source", sadr)
		if saddr, err := net.ResolveTCPAddr("tcp", sadr); err != nil {
			log.Fatal(err)
		} else {
			d = net.Dialer{LocalAddr: saddr}
		}
	}
	c.limiter <- 0
	conn, err := d.DialContext(ctx, "tcp", *c.addr)
	<-c.limiter
	if err != nil {
		//log.Println("Connect", err)
		atomic.AddUint64(&stats.FailedConnects, 1)
		return
	}
	defer conn.Close()

	// Transfer the context deadline to the connection
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	buf := make([]byte, 4096)
	for ok := true; ok; ok = *c.keep {
		len, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Read err:", err)
			atomic.AddUint64(&stats.FailedReads, 1)
			return
		}
		host := string(buf[:len])
		hostch <- host
	}
}

func (c *config) udpConnect(ctx context.Context) {
	defer c.wg.Done()

	var saddr *net.UDPAddr
	if c.rndip != nil {
		sadr := fmt.Sprintf("%s:0", c.rndip.GetIPString())
		var err error
		if saddr, err = net.ResolveUDPAddr("udp", sadr); err != nil {
			log.Fatal(err)
			return
		}
	}

	raddr, err := net.ResolveUDPAddr("udp", *c.addr)
	if err != nil {
		atomic.AddUint64(&stats.FailedConnects, 1)
		return
	}

	c.limiter <- 0
	conn, err := net.DialUDP("udp", saddr, raddr)
	if err != nil {
		<-c.limiter
		atomic.AddUint64(&stats.FailedConnects, 1)
		return
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	if _, err = conn.Write([]byte("Hello")); err != nil {
		<-c.limiter
		atomic.AddUint64(&stats.FailedConnects, 1)
		return
	}
	buf := make([]byte, 4096)
	len, err := conn.Read(buf)
	<-c.limiter
	if err != nil {
		atomic.AddUint64(&stats.FailedReads, 1)
		return
	}
	host := string(buf[:len])
	hostch <- host
}


// ----------------------------------------------------------------------
// Stats

type statistics struct {
	Hostmap        map[string]int `json:"hosts"`
	Connects       int            `json:"connects"`
	FailedConnects uint64         `json:"failed_connects"`
	FailedReads    uint64         `json:"failed_reads"`
	Start          time.Time      `json:"start_time"`
	Timeout        time.Duration  `json:"timeout"`
	Duration       time.Duration  `json:"duration"`
}

var hostch = make(chan string, 100)
var stats = statistics{
	Hostmap: make(map[string]int),
}

func statsWorker(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		h := <-hostch
		if h == "QUIT!" {
			return
		}
		if val, ok := stats.Hostmap[h]; ok {
			stats.Hostmap[h] = (val + 1)
		} else {
			stats.Hostmap[h] = 1
		}
	}
}
