package bhandlers

import (
	"bytes"
	"fmt"
	"net/http"
	//"net/request"
	//"encoding/hex"
	"encoding/json"
	//"log"
	"strings"
	//"github.com/klyed/hivesmartchain/execution"
	//"github.com/klyed/hive-go"
	//"github.com/klyed/hive-go/transactions"
	types "github.com/klyed/hive-go/types"
	//"error"
)

//var (
//  response = fmt.Printf("")
//  error = nil
//)

// Generated by curl-to-Go: https://mholt.github.io/curl-to-go

// curl -X POST -H "Content-Type: application/json" -H "Authorization: Bearer b7d03a6947b217efb6f3ec3bd3504582" -d '{"type":"A","name":"www","data":"162.10.66.0","priority":null,"port":null,"weight":null}' "https://api.digitalocean.com/v2/domains/example.com/records"
/*
type Payload struct {
	id       string      `json:"type"`
	action   string      `json:"name"`
	method   string      `json:"method"`
	data     interface{} `json:"data"`
}

data := Payload{
// fill struct
}
payloadBytes, err := json.Marshal(data)
if err != nil {
	// handle err
}
body := bytes.NewReader(payloadBytes)

req, err := http.NewRequest("POST", "https://api.digitalocean.com/v2/domains/example.com/records", body)
if err != nil {
	// handle err
}
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer b7d03a6947b217efb6f3ec3bd3504582")

resp, err := http.DefaultClient.Do(req)
if err != nil {
	// handle err
}
defer resp.Body.Close()
*/

// Generated by curl-to-Go: https://mholt.github.io/curl-to-go

// curl -X POST -d '{"method": "names", "id": "foo", "params": ["loves"]}' http://0.0.0.0:26658
func Curl(id string, sender string, action string, method string, params map[string]interface{}) (interface{}, error) {

	type Payload struct {
		//id       string      `json:"type"`
		action string                 `params:"name"`
		method string                 `params:"method"`
		data   map[string]interface{} `params:"data"`
	}

	//data := Payload{
	// fill struct
	//}
	payloadBytes, err := json.Marshal(params)
	if err != nil {
		// handle err
	}
	body := bytes.NewReader(payloadBytes)

	//bodyContent := []interface{string}{id: id, action: action, method: method, params: params}
	//body := strings.NewReader(bodyContent)
	req, err := http.NewRequest("POST", "http://0.0.0.0:26658", body)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		// handle err
		return fmt.Printf("ERROR: %v", err)
	}
	//req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// handle err
		return fmt.Printf("ERROR: %v", err)
	}
	fmt.Printf("Response: %v", resp)
	defer resp.Body.Close()
	return resp, err
}

func CustomJSON(block uint32, tx *types.Transaction, op *types.CustomJSONOperation) (int, error) {
	//fmt.Printf("\n\nHIVE --(custom_json op)-> HSC: Op: %v", op)
	sender := op.RequiredAuths[0]
	json := op.JSON
	jsonParsed := []interface{}{json}
	action := jsonParsed[0]
	method := jsonParsed[1]
	data := []interface{}{jsonParsed[2]}
	return fmt.Printf("HIVE --(custom_json)-> HSC: Sender: %v - Action: %v - Method: %v - Data: %v)", sender, action, method, data)
	//execution
	//return response, error
}

func Transfer(block uint32, tx *types.Transaction, op *types.TransferOperation) (int, error) {
	sender := op.From
	//receiver := op.To
	amount := op.Amount
	amountsplit := strings.Fields(amount)
	value := amountsplit[0]
	coin := amountsplit[1]
	//memo := op.Memo
	return fmt.Printf("HIVE --(transfer)-> HSC: Sender: %v - Amount: %v or %v %v)", sender, amount, value, coin)
	//return response, error
}
