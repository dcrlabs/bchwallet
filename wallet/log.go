// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"github.com/dcrlabs/bchwallet/waddrmgr"
	"github.com/dcrlabs/bchwallet/walletdb/migration"
	"github.com/dcrlabs/bchwallet/wtxmgr"
	"github.com/gcash/bchlog"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
var log bchlog.Logger

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output.  Logging output is disabled
// by default until either UseLogger or SetLogWriter are called.
func DisableLog() {
	UseLogger(bchlog.Disabled)
}

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using bchlog.
func UseLogger(logger bchlog.Logger) {
	log = logger

	migration.UseLogger(logger)
	waddrmgr.UseLogger(logger)
	wtxmgr.UseLogger(logger)
}

// LogClosure is a closure that can be printed with %v to be used to
// generate expensive-to-create data for a detailed log level and avoid doing
// the work if the data isn't printed.
type logClosure func() string

// String invokes the log closure and returns the results string.
func (c logClosure) String() string {
	return c()
}

// newLogClosure returns a new closure over the passed function which allows
// it to be used as a parameter in a logging function that is only invoked when
// the logging level is such that the message will actually be logged.
func newLogClosure(c func() string) logClosure {
	return logClosure(c)
}

// pickNoun returns the singular or plural form of a noun depending
// on the count n.
func pickNoun(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
