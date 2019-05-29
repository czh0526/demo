package console

/*
	mainly message
*/
type jsonSuccessResponse struct {
	Version string      `json:"version"`
	Id      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result"`
}

type jsonErrResponse struct {
	Version string      `json:"version"`
	Id      interface{} `json:"id,omitempty"`
	Error   jsonError   `json:"error"`
}

type jsonNotification struct {
	Version string           `json:"verion"`
	Method  string           `json:"method"`
	Params  jsonSubscription `json:"params"`
}

/*
	other message components
*/

type jsonError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type jsonSubscription struct {
	Subscription string      `json:"subscription"`
	Result       interface{} `json:"result,omitempty"`
}
