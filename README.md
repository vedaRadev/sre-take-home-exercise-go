Ryan Armstrong 2025

# Fetch SRE Challenge

## About

TODO description of the program

## Installation

TODO installation info

## Fixes and Improvements

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
