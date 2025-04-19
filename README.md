# sysmonitor

`sysmonitor` is a Go package designed to monitor the health of MySQL, Redis, and other system components. It provides health checkers for MySQL and Redis databases and allows you to monitor their status at regular intervals. You can also extend it to add more health checkers for other system components.

## Features

- MySQL Health Checker
- Redis Health Checker
- Configurable heartbeat interval
- Context-based cancellation for graceful shutdown
- Supports logging and resource monitoring (optional)

## Installation

You can import and install `sysmonitor` into your project using `go get`.

```bash
go get github.com/jiggyjango/sysmonitor
