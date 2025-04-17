# Fetch SRE Challenge

## About

This program takes a yaml configuration containing endpoint configs and checks
domain availability every 15 seconds. Note that currently this does _not_ mean a
report is printed every 15 seconds, but that a new check cycle is started every
15 seconds. A report will be printed at the end of each check cycle.

Endpoints are considered available if
- They return a status code in the 200-range.
- They respond within 500ms.

Note that domain availability is cumulative, meaning if all endpoints of a
domain succeed in cycle A but fail in cycle B, then the domain has an
availability of 50%. 

A valid YAML endpoint configuration is as follows:
```
name (string, required) - free-text name describing the HTTP endpoint
url (string, required) - url of the endpoint, may be HTTP or HTTPS
method (string, optional) - HTTP method, defaults to GET
headers (dictionary, optional) - request headers
body (string, optional) - request body
```

## Installation

Requirements
- [Go](https://go.dev/doc/install) version 1.16 or later

1. Clone this repository.
2. Build with `go build` or run with `go run .`

## Usage
Note: You can also run this with `go run . <args>`.
```
() = required, [] = optional

fetch-sre-exercise (endpoint_config.yaml)
    [--no-req-timeout] [--debug-logs] [--max-domain-concurrency <int>]
```
- `--no-req-timeout` - Disable the 500ms request deadline for debugging purposes. Default: false.
- `--debug-logs` - Enable debug logs. Default: false.
- `--max-domain-concurrency <int>` - set max number of concurrent requests per domain per check cycle (must be > 0). Default: 10.

Terminate execution via `CTRL-C`.

## Things I thought about but didn't do for the sake of time
- Publish the Go module.
- Add a little CI/CD pipeline with Github Actions, including test runs
- Cleaner central synchronized logger API (see fixes and improvements as to what
  this is)
- Echo logs to file (would be rolled into the central synchronized logger)

## Things I didn't do for other reasons

- Break out functions into separate files/packages, mostly because this program
  is so small and I feel like breaking stuff into separate files would make the
  code just harder to follow. (I did eventually break out SyncedPrinter into its
  own file because it got large enough to warrant it, and it was obvious it was
  its own self-contained entity.)

## Fixes and Improvements
Note: All fixes and improvements here are listed in the order they were added.
Some build off changes made for previous fixes and improvements.

### Added additional logging
##### Discovery
N/A

##### Why?
To aid in the identification of errors.
An even more robust logging system would also flush logs to a file but that
would usually require a custom logging system.
I haven't explored if Go has a built-in method of easily logging to
stdout/stderr AND a file in one call or in a centralized manner.

### Default endpoint.Method to "GET"
##### Discovery
Issue discovered with the aid of additional logging mechanisms.
When a request to an endpoint failed (errored or returned with a non-200-range
status), I saw that the `endpoint.Method` was empty.

##### Why?
Although I think Go may default the request method to GET if an empty string is
provided, explicitly defaulting the Method to a non-empty `"GET"` string more
clearly conveys intent.
It will also potentially shield against possibly difficult-to-track-down issue
if the behavior of `http.NewRequest` changes how it handles an empty-string
`""` method between Go versions (unlikely but has happened in libraries before).

### Create a reader for `endpoint.Body` instead of a JSON-serialized `endpoint`
##### Discovery
I initially noticed that we were marshalling the entire `endpoint` into a reader
for the request body instead of the `endpoint.Body` field while walking through
the code in a debugger.
I also noticed that an endpoint was responding with a 422 status code (courtesy
of additional logging I added), but the name of that endpoint in the YAML file
implied that the status should've been in the 200 range.

##### Why?
The YAML explanation in the PDF explicitly states that the `encoding.Body` is
the HTTP body to include in the request, and that if it's present it will be a
valid JSON-encoded string.
Therefore, I could just make a reader from the bytes of the `encoding.Body`
string without having to worry about marshalling any JSON.
Additionally, the documentation for `http.NewRequestWithContext` (which
`http.NewRequest` wraps) states that Body will be set to `NoBody` and
the request's ContentLength will be set to 0 if the `bytes.Reader` has no
content, which should be true in the case of a reader for an empty string.

### Add a timeout context with a 500ms deadline to the request
##### Discovery
I was able to tell just by looking at the `checkHealth` that we weren't creating
a request with a context containing a timeout. At this point I've also been
working with this function for a bit and I've examined the program flow with a
debugger several times.

##### Why?
The PDF explicitly states that endpoints that do not respond within 500ms should
be considered unavailable.
Creating a request with a context that contains a deadline will allow Go to
abort the request (and any Goroutines that may have been spawned in the process
of making it, provided they have the deadline context and are well-behaved) if
the deadline is exceeded.
There's no point in waiting for a response past 500ms so we'll just continue on
and make sure to free the context's resources with the deferred `cancel()` call.

##### Additional Note
I also added the ability to disable the 500ms request timeout by passing
`--no-req-timeout` as a command-line argument for the purposes of debugging.

### Run domain requests concurrently, limiting the number of in-flight requests per-domain
##### Discovery
Just looking at the code I was able to see that it was running requests serially
instead of concurrently.

##### Why?
Even though this doesn't directly address the requirement that check cycles
should be run every 15 seconds, the intent is to prepare for an upcoming change
that _will_ address it.
As a thought exercise, assume that we _are_ kicking off check cycles every 15
seconds as required and that we have many endpoints, a not-insignificant number
of which are timing out at the 500ms threshold.
We will not be able to visit endpoints quickly enough to obtain a very practical
availability report every 15 seconds.
Making requests concurrently will allow us to chew through checks more quickly
and hopefully avoid that scenario.

I limited the number of in-flight requests _per domain_ so that we can still
make as many checks as possible while avoiding undue stress to systems since
this is an availability check, not a stress test.
Also I'm using a few different domains in my custom `test.yaml` and I don't
want to be rate limited or, worse, IP banned for request spamming.

### Make `extractDomain` port-agnostic
##### Discovery
I had a hunch that the function wasn't stripping the port number so I wrote a
simple unit test for it and tested a few cases. Lo and behold, it wasn't
stripping the port.

##### Why?
Well, the requirements specifically state that we must ignore port numbers when
determining the domain. But, more technically, there's not anything stopping
someone from setting up a two different services to run on the same domain but
different ports (for example, maybe a public API runs on port X but static
content is served from port Y or something, I don't know this is a bad example).

### Launch a goroutine to run the check cycle and availability report every 15 seconds, and also add a synced printing mechanism
##### Discovery AND Why? (check cycle stuff)
At this point I've run the code enough times and looked at it for long enough to
see that we would run a check cycle, wait for it to finish, _then_ wait 15
seconds to run the next checks.
That means that if there are many, many endpoints or if we're timing out at the
max 500ms on many requests, we might not launch a new check cycle for > 15 seconds.
What we _really_ want is to _start_ a check cycle, then launch a new check cycle
15 seconds later _regardless of whether the previous check cycle fully finished_.

##### Why? (synced printing mechanism)
It's technically possible that we launch another check cycle before we finish
our previous check cycle.
Given that I'm still just kind of spitting out tons of logs for debugging, it's
likely that we'll print some information in the middle of printing an
availability report, cluttering the output and making it hard to read.
The little thread safe / synced logger I hacked together just ensures that
information still looks relatively organized in the console.

Also having a little centralized logger/printer makes it easy to enable/disable
those debug logs (or implement some sort of log level system + filter) later on.
Also also if I want to later on separate the availability printing from the
check cycles so that we always print a report every 15 seconds instead of just
whenever a check cycle ends (which takes an unknown amount of time), having this
in place makes doing that a little easier.

### Add debug and normal logging/printing, update arg parsing, debug logs are disabled by default
##### Discovery
N/A

##### Why?
The debug logging I added clutters the screen and makes finding information
harder. I implemented a slightly smarter arg parser since the previous way I was
doing it wasn't flexible enough to support multiple flags in a convenient manner.

### Remove `io.ioutil.ReadFile` in favor of `os.ReadFile`
##### Discovery
LSP alerted me to the fact this was deprecated since Go 1.16. I just didn't get
around to changing it until now.

##### Why?
"Cleanliness." `io.ioutil.ReadFile` just wraps `os.ReadFile` anyway. Features
that are deprecated are usually likely to be removed in future versions.

### Allow user to specify the max concurrency per domain per check cycle
##### Discovery
N/A

##### Why?
Convenience, debugging
