// reference https://github.com/trojan-gfw/igniter-go-libs/blob/master/tun2socks/tun2socks.go
package main

import (
	"C"

	"context"
	"io"
	"net"
	"strings"
	"time"

	//"github.com/Trojan-Qt5/go-tun2socks/common/log"
	_ "github.com/Trojan-Qt5/go-tun2socks/common/log/simple"
	"github.com/Trojan-Qt5/go-tun2socks/core"
	"github.com/Trojan-Qt5/go-tun2socks/proxy/socks"
	"github.com/Trojan-Qt5/go-tun2socks/tun"

	_ "github.com/p4gefau1t/trojan-go/build"
	"github.com/p4gefau1t/trojan-go/common"
	"github.com/p4gefau1t/trojan-go/log"

	"github.com/Trojan-Qt5/go-shadowsocks2/cmd/shadowsocks"

	v2ray "github.com/Trojan-Qt5/v2ray-go/core"
)
import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/bit-rocket/open-snell/components/snell"
	"github.com/p4gefau1t/trojan-go/conf"
	"github.com/p4gefau1t/trojan-go/proxy"
)

const (
	MTU = 1500
)

var (
	client     common.Runnable
	lwipWriter io.Writer
	tunDev     io.ReadWriteCloser
	ctx        context.Context
	cancel     context.CancelFunc
	isRunning  bool = false

	isTrojanGoRunning bool = false
	isSnellGoRunning  bool = false
)

//export is_tun2socks_running
func is_tun2socks_running() bool {
	return isRunning
}

//export stop_tun2socks
func stop_tun2socks() {
	log.Info("Stopping tun2socks")

	isRunning = false

	err := tunDev.Close()
	if err != nil {
		log.Fatalf("failed to close tun device: %v", err)
	}

	cancel()
}

//export run_tun2socks
func run_tun2socks(tunName *C.char, tunAddr *C.char, tunGw *C.char, tunDns *C.char, proxyServer *C.char) {

	// Open the tun device.
	dnsServers := strings.Split(C.GoString(tunDns), ",")
	var err error
	tunDev, err = tun.OpenTunDevice(C.GoString(tunName), C.GoString(tunAddr), C.GoString(tunGw), "255.255.255.0", dnsServers)
	if err != nil {
		log.Error("open tun device err:%v", err)
	}

	// Setup TCP/IP stack.
	lwipWriter := core.NewLWIPStack().(io.Writer)

	// Register tun2socks connection handlers.
	proxyAddr, err := net.ResolveTCPAddr("tcp", C.GoString(proxyServer))
	proxyHost := proxyAddr.IP.String()
	proxyPort := uint16(proxyAddr.Port)
	if err != nil {
		log.Info("invalid proxy server address: %v", err)
	}
	core.RegisterTCPConnHandler(socks.NewTCPHandler(proxyHost, proxyPort, nil))
	core.RegisterUDPConnHandler(socks.NewUDPHandler(proxyHost, proxyPort, 1*time.Minute, nil, nil))

	// Register an output callback to write packets output from lwip stack to tun
	// device, output function should be set before input any packets.
	core.RegisterOutputFn(func(data []byte) (int, error) {
		return tunDev.Write(data)
	})

	ctx, cancel = context.WithCancel(context.Background())

	// Copy packets from tun device to lwip stack, it's the main loop.
	go func(ctx context.Context) {
		_, err := io.CopyBuffer(lwipWriter, tunDev, make([]byte, MTU))
		if err != nil {
			log.Info(err.Error())
		}
	}(ctx)

	log.Info("Running tun2socks")

	isRunning = true

	<-ctx.Done()
}

//export startShadowsocksGo
func startShadowsocksGo(ClientAddr *C.char, ServerAddr *C.char, Cipher *C.char, Password *C.char, Plugin *C.char, PluginOptions *C.char, EnableAPI bool, APIAddress *C.char) {
	shadowsocks.StartGoShadowsocks(C.GoString(ClientAddr), C.GoString(ServerAddr), C.GoString(Cipher), C.GoString(Password), C.GoString(Plugin), C.GoString(PluginOptions), EnableAPI, C.GoString(APIAddress))
}

//export stopShadowsocksGo
func stopShadowsocksGo() {
	shadowsocks.StopGoShadowsocks()
}

//export startTrojanGo
func startTrojanGo(filename *C.char) {
	if client != nil {
		log.Info("Client is already running")
		return
	}
	log.Info("Running client, config file:", C.GoString(filename))
	configBytes, err := ioutil.ReadFile(C.GoString(filename))
	if err != nil {
		log.Error("failed to read file", err)
	}
	config, err := conf.ParseJSON(configBytes)
	if err != nil {
		log.Error("error", err)
		return
	}
	client, err = proxy.NewProxy(config)
	if err != nil {
		log.Error("error", err)
		return
	}
	go client.Run()
	log.Info("trojan launched")
	isTrojanGoRunning = true
}

//export stopTrojanGo
func stopTrojanGo() {
	if isTrojanGoRunning {
		log.Info("Stopping client")
		if client != nil {
			client.Close()
			client = nil
		}
		log.Info("Stopped")
		isTrojanGoRunning = false
	}
}

//export getTrojanGoVersion
func getTrojanGoVersion() *C.char {
	return C.CString(common.Version)
}

//export testV2rayGo
func testV2rayGo(configFile *C.char) (bool, *C.char) {
	status, err := v2ray.TestV2ray(C.GoString(configFile))
	return status, C.CString(err)
}

//export startV2rayGo
func startV2rayGo(configFile *C.char) {
	v2ray.StartV2ray(C.GoString(configFile))
}

//export stopV2rayGo
func stopV2rayGo() {
	v2ray.StopV2ray()
}

type SnellConfig struct {
	SnellAPI   SnellAPI  `json:"api"`
	LocalAddr  string    `json:"local_addr"`
	LocalPort  int       `json:"local_port"`
	PSK        string    `json:"psk"`
	RemoteAddr string    `json:"remote_addr"`
	RemotePort int       `json:"remote_port"`
	Obfs       SnellObfs `json:"obfs"`
}
type SnellAPI struct {
	ApiAddr string `json:"api_addr"`
	ApiPort int    `json:"api_port"`
	Enabled bool   `json:"enabled"`
}
type SnellObfs struct {
	Host string `json:"obfs_host"`
	Type string `json:"obfs_type"`
}

//export startSnellGo
func startSnellGo(configFile *C.char) {
	if isSnellGoRunning {
		log.Info("snell go is already running")
		return
	}

	log.Info("Running snell client, config file:", C.GoString(configFile))
	configBytes, err := ioutil.ReadFile(C.GoString(configFile))
	if err != nil {
		log.Errorf("failed to read file:%v", err)
		return
	}
	cf := SnellConfig{}
	if err := json.Unmarshal(configBytes, &cf); err != nil {
		log.Errorf("load snell json config err:%v", err)
		return
	}
	if err := snell.StartGoSnell(
		fmt.Sprintf("%s:%d", cf.LocalAddr, cf.LocalPort),
		fmt.Sprintf("%s:%d", cf.RemoteAddr, cf.RemotePort),
		cf.Obfs.Type, cf.Obfs.Host, cf.PSK, true, cf.SnellAPI.Enabled,
		fmt.Sprintf("%s:%d", cf.SnellAPI.ApiAddr, cf.SnellAPI.ApiPort),
	); err != nil {
		log.Errorf("failed to start go snell:%v", err)
		return
	}
	log.Info("start snell go client ok")
	isSnellGoRunning = true
}

//export stopSnellGo
func stopSnellGo() {
	if !isSnellGoRunning {
		log.Warn("try to stop while snell go not running")
		return
	}
	snell.StopGoSnell()
	isSnellGoRunning = false
	log.Info("stop snell go client")
}

func main() {
}
