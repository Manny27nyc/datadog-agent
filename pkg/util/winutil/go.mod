module github.com/DataDog/datadog-agent/pkg/util/winutil

go 1.16

replace github.com/DataDog/datadog-agent/pkg/util/log => ../log

replace github.com/DataDog/datadog-agent/pkg/util/scrubber => ../scrubber

require (
	github.com/DataDog/datadog-agent/pkg/util/log v0.32.0-rc.6
	github.com/stretchr/testify v1.7.0
	golang.org/x/sys v0.1.0
)
