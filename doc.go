// Package bamboohr is a generic, policy-free Go client for the BambooHR API.
//
// It provides authentication, transport, typed errors, retry/backoff, and
// pagination over BambooHR's REST API, plus read access to employee data and
// metadata. It carries no organization-specific knowledge: callers name the
// fields they want and interpret the values themselves.
package bamboohr
