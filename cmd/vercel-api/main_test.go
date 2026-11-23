package main

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestOnlySupportedJSONRoutes(t *testing.T) {
    tests := []struct {
        path string
        want bool
    }{
        {"/instance/create", true},
        {"/send/text", true},
        {"/group/list", true},
        {"/server/ok", true},
        {"/license/status", true},
        {"/license/register", true},
        {"/license/activate", true},
        {"/manager/login", false},
        {"/ws", false},
        {"/assets/file.js", false},
        {"/v1/register/auto", false},
        {"/passkey/response", false},
        {"/not-supported", false},
    }
    for _, tc := range tests {
        if got := allowPath(tc.path);got != tc.want {
            t.Errorf("allowPath(%q) = %v; want %v", tc.path, got, tc.want)
        }
    }
}

func TestUnauthorizedBeforeDatabaseAccess(t *testing.T) {
    g:=&gateway{key:"a-long-test-key"}
    for _,path:=range []string{"/instance/create","/bridge/jobs/00000000-0000-0000-0000-000000000000"}{
        req:=httptest.NewRequest(http.MethodGet,path,nil)
        rec:=httptest.NewRecorder()
        g.ServeHTTP(rec,req)
        if rec.Code!=http.StatusUnauthorized {
            t.Errorf("%s: got %d, want 401",path,rec.Code)
        }
    }
}
