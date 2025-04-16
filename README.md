Ryan Armstrong 2025

# Fetch SRE Challenge

## About

TODO description of the program

## Installation

TODO installation info

## Fixes and Improvements
Note: All fixes and improvements here are listed in the order they were added.

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
