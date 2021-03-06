package secrethub

import (
	"github.com/alecthomas/kingpin"
	"github.com/secrethub/secrethub-cli/internals/cli"
)

// FlagRegisterer allows others to register flags on it.
type FlagRegisterer interface {
	Flag(name, help string) *cli.Flag
}

func registerTimestampFlag(r FlagRegisterer) *kingpin.FlagClause {
	return r.Flag("timestamp", "Show timestamps formatted to RFC3339 instead of human readable durations.").Short('T')
}

func registerForceFlag(r FlagRegisterer) *kingpin.FlagClause {
	return r.Flag("force", "Ignore confirmation and fail instead of prompt for missing arguments.").Short('f')
}
