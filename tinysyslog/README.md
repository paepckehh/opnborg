# tinysyslog [![Go Report Card](https://goreportcard.com/badge/github.com/alexferl/tinysyslog)](https://goreportcard.com/report/github.com/alexferl/tinysyslog) [![codecov](https://codecov.io/gh/alexferl/tinysyslog/branch/master/graph/badge.svg)](https://codecov.io/gh/alexferl/tinysyslog)

A tiny and simple syslog server with log rotation. tinysyslog was born out of the need for a tiny, easy to set up and 
use syslog server that simply writes every incoming log (in [RFC 5424](https://datatracker.ietf.org/doc/html/rfc5424) format **only**) to a file that is automatically rotated, 
to stdout or stderr (mostly for Docker) and or to Elasticsearch.
tinysyslog is based on [go-syslog](https://github.com/mcuadros/go-syslog) and [lumberjack](https://github.com/natefinch/lumberjack).

