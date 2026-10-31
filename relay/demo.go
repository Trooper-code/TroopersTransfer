package relay

// NewDemo returns a Host that runs nothing and just reports st. It exists so
// the UI can be rendered in known states for tests and README screenshots.
func NewDemo(st Status) *Host {
	return &Host{opts: Options{Port: st.Port, Password: st.Password}, status: st}
}

// DemoManualForwardProblem returns the "couldn't open the ports
// automatically" guide for the given network, for tests and screenshots.
func DemoManualForwardProblem(localIP, routerIP string, port int) *Problem {
	return problemNoUPnP(netFacts{localIP: localIP, routerIP: routerIP, ports: Ports(port)},
		"no UPnP or NAT-PMP router answered: no UPnP internet gateway found on this network")
}
