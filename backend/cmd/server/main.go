// Command server is the Dexiask backend: an HTTP service that streams chat over
// SSE by bridging to the Claude engine, persists conversations in Postgres,
// handles file-upload attachments, reverse-proxies the indexer control-plane,
// and runs a Slack Socket Mode bot.
package main

import (
	"go.uber.org/fx"
)

func main() {
	fx.New(
		ConfigModule,
		InfrastructureModule,
		AgentModule,
		RepositoryModule,
		AuthModule,
		ServiceModule,
		HandlerModule,
		SlackModule,
		ServerModule,
	).Run()
}
