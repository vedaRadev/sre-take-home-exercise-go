package main

import "testing"

func TestExtractDomain(t *testing.T) {
	tests := []struct{
		name string
		url string
		expected string
	}{
		{
			name: "basic test",
			url: "https://www.helloworld.com",
			expected: "www.helloworld.com",
		},
		{
			name: "strips path",
			url: "https://blog.boot.dev/about",
			expected: "blog.boot.dev",
		},
		{
			name: "port agnostic",
			url: "https://localhost:8080",
			expected: "localhost",
		},
		{
			name: "port agnostic AND strips path",
			url: "https://localhost:8080/this/is/a/path",
			expected: "localhost",
		},
		{
			name: "no protocol",
			url: "www.helloworld.com",
			expected: "www.helloworld.com",
		},
	}

	for i, testCase := range tests {
		t.Run(testCase.name, func (t *testing.T) {
			actual := extractDomain(testCase.url)
			if actual != testCase.expected {
				t.Errorf("Test %v - %v FAIL: expected %v but got %v", i, testCase.name, testCase.expected, actual)
			}
		})
	}
}
