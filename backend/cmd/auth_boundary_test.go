package main

import (
	"os"
	"strings"
	"testing"
)

func TestBankingExecutableDoesNotRegisterCredentialRoutes(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/register", "/login", "/logout", "/change-password", "/forgot-password", "/reset-password"} {
		if strings.Contains(string(source), `.Post("`+route+`"`) {
			t.Errorf("Banking must not register Identity-owned route %s", route)
		}
	}
}
