package svc

import (
	"fmt"
	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGETUserEndpointVariousData(t *testing.T) {
	type TestStruct struct {
		testDataName         string
		endPoint             string
		expectedStatusCode   int
		responseBody         string
		expectedResponseBody string
	}

	var tests = []TestStruct{
		{"validEndPointEmptyUser", `/user/`, 200, "", "missing user in context"},
		{"validUserEndPointGetUser", `/user/user1`, 200, "", ""},
		{"validUserEndPointListUsers", `/users`, 200, "", ""},
	}

	for _, testCase := range tests{
		t.Run(testCase.testDataName, func(t *testing.T){
			rr:= sendRequest(t, "GET", testCase.endPoint, "")
			fmt.Println("hello i am here")
			fmt.Println(rr)
		})
	}
}

func sendRequest(t *testing.T, method, endpoint, data string) *httptest.ResponseRecorder {
	var reader = strings.NewReader(data)
	req, err := http.NewRequest(method, endpoint, reader)
	assert.Nil(t, err)
    NewService()
	r := chi.NewRouter()
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}
