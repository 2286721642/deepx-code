package web

import (
	"net"
	"testing"
	"time"
)

// fakeConn 实现一个只暴露给定 LocalAddr 的 net.Conn 桩,用于测试 outboundIP。
type fakeConn struct{ local net.Addr }

func (f *fakeConn) Read([]byte) (int, error)         { return 0, nil }
func (f *fakeConn) Write([]byte) (int, error)        { return 0, nil }
func (f *fakeConn) Close() error                     { return nil }
func (f *fakeConn) LocalAddr() net.Addr              { return f.local }
func (f *fakeConn) RemoteAddr() net.Addr             { return nil }
func (f *fakeConn) SetDeadline(time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// udpAddr 把 "host:port" 转成 *net.UDPAddr 桩,模拟 UDP dial 的 LocalAddr。
func udpAddr(hostport string) net.Addr {
	a, err := net.ResolveUDPAddr("udp", hostport)
	if err != nil {
		panic(err)
	}
	return a
}

// TestOutboundIP 验证通过 dial 外部地址选出默认路由出口 IP 的逻辑。
func TestOutboundIP(t *testing.T) {
	tests := []struct {
		name    string
		dial    func() (net.Conn, error)
		wantIP  string
		wantOK  bool
	}{
		{
			name: "出口为真实局域网网卡 -> 返回它",
			dial: func() (net.Conn, error) {
				return &fakeConn{local: udpAddr("192.168.1.50:54321")}, nil
			},
			wantIP: "192.168.1.50",
			wantOK: true,
		},
		{
			name: "dial 失败 -> 回退(false)",
			dial: func() (net.Conn, error) {
				return nil, net.ErrWriteToConnected
			},
			wantIP: "",
			wantOK: false,
		},
		{
			name: "出口为回环 -> 回退(false)",
			dial: func() (net.Conn, error) {
				return &fakeConn{local: udpAddr("127.0.0.1:54321")}, nil
			},
			wantIP: "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, ok := outboundIP(tt.dial)
			if ok != tt.wantOK || ip != tt.wantIP {
				t.Fatalf("outboundIP() = (%q, %v), want (%q, %v)", ip, ok, tt.wantIP, tt.wantOK)
			}
		})
	}
}


// mustIPNet 把 "ip/prefix" 字符串转成 *net.IPNet,用于构造测试地址。
// 注意:net.ParseCIDR 返回的 *net.IPNet.IP 是网络号(主机位清零),而 net.InterfaceAddrs()
// 在真实运行时的 *net.IPNet.IP 是接口主机地址。这里把主机地址写回 IPNet.IP,
// 以精确模拟真实网卡地址的形制(否则 .String() 会得到 192.168.1.0 而非 192.168.1.50)。
func mustIPNet(cidr string) net.Addr {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	ipnet.IP = ip
	return ipnet
}

// TestSelectHostIP 验证从多网卡地址中挑选访问 URL 用的 IP:
// 必须跳过链路本地 169.254.x.x,优先返回真实局域网地址。
func TestSelectHostIP(t *testing.T) {
	tests := []struct {
		name   string
		addrs  []net.Addr
		want   string
	}{
		{
			name: "链路本地排在前,真实网卡在后 -> 取真实网卡",
			addrs: []net.Addr{
				mustIPNet("169.254.8.253/16"), // 出问题的 APIPA 地址
				mustIPNet("192.168.1.50/24"),  // 用户真正的局域网 IP
			},
			want: "192.168.1.50",
		},
		{
			name: "只有链路本地 -> 退回它(而非 127.0.0.1)",
			addrs: []net.Addr{
				mustIPNet("169.254.8.253/16"),
			},
			want: "169.254.8.253",
		},
		{
			name: "回环地址被忽略,取真实网卡",
			addrs: []net.Addr{
				mustIPNet("127.0.0.1/8"),
				mustIPNet("10.0.0.5/8"),
			},
			want: "10.0.0.5",
		},
		{
			name: "虚拟网卡(链路本地)与真实网卡并存,取真实(排序在任意位置)",
			addrs: []net.Addr{
				mustIPNet("172.16.3.1/12"),   // 真实局域网
				mustIPNet("169.254.1.1/16"),  // Hyper-V/Docker 虚拟网卡
			},
			want: "172.16.3.1",
		},
		{
			name: "只有回环与 IPv6 链路本地 -> 退回 127.0.0.1",
			addrs: []net.Addr{
				mustIPNet("127.0.0.1/8"),
				mustIPNet("fe80::1/64"),
			},
			want: "127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectHostIP(tt.addrs); got != tt.want {
				t.Fatalf("selectHostIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
