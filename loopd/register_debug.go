// +build dev

package loopd

import (
	"github.com/lightninglabs/loop/looprpc"
	"gopkg.in/macaroon-bakery.v2/bakery"
)

var (
	debugRequiredPermissions = map[string][]bakery.Op{
		"/looprpc.DebugClient/ForceAutoLoop": {{
			Entity: "debug",
			Action: "write",
		}},
	}

	debugPermissions = []bakery.Op{
		{
			Entity: "debug",
			Action: "write",
		},
	}
)

// registerDebugServer registers the debug server.
func (d *Daemon) registerDebugServer() {
	looprpc.RegisterDebugClientServer(d.grpcServer, d)
}
