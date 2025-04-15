Ryan Armstrong 2025

# Fetch SRE Challenge

## About

TODO description of the program

## Installation

TODO installation info

## Improvements

- Added additional logging to aid in identification of errors.

## Fixes

### Default endpoint.Method to "GET"
##### Discovery
Issue discovered with the aid of additional logging mechanisms.
When a request to an endpoint failed (errored or returned with a non-200-range
status), I saw that the `endpoint.Method` was empty.

##### Why?
Although I think Go may default the request method to GET if an empty string is
provided, explicitly defaulting the Method to a non-empty `"GET"` string more
clearly conveys intent.
It will also potentially shield a possibly difficult-to-track-down issue if the
behavior of `http.NewRequest` changes how it handles an empty-string `""` method
between Go versions (unlikely but has happened in libraries before).
