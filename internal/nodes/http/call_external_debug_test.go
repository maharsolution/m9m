package http_test

import (
  "encoding/json"
  "fmt"
  "os"
  "testing"
  "time"

  httpnode "github.com/neul-labs/m9m/internal/nodes/http"
  "github.com/neul-labs/m9m/internal/model"
  "github.com/neul-labs/m9m/internal/nodes/transform"
)

func agentLog(hid, loc, msg string, data map[string]interface{}) {
  f, err := os.OpenFile(`c:\Users\mahar\OneDrive\workspace\m9m\debug-31457b.log`, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil { return }
  defer f.Close()
  payload := map[string]interface{}{"sessionId":"31457b","hypothesisId":hid,"location":loc,"message":msg,"data":data,"timestamp":time.Now().UnixMilli(),"runId":"pre-fix"}
  b,_ := json.Marshal(payload)
  _,_ = f.Write(append(b,'\n'))
}

func TestDebug_IsAsyncAndCallExternalURL(t *testing.T) {
  // Current m9m HTTP Request wire shape (statusCode/headers/body/json)
  // vs n8n-flattened body. Proves why isAsync takes the false branch live.
  m9mHTTPShape := model.DataItem{JSON: map[string]interface{}{
    "statusCode": 200,
    "headers":    map[string]interface{}{"Content-Type": "application/json"},
    "body":       `{"data":{"attributes":{"route":"async"}}}`,
    "json": map[string]interface{}{
      "data": map[string]interface{}{
        "matched":       true,
        "target_rule":   "service-rules-order",
        "attributes":    map[string]interface{}{"route": "async"},
      },
    },
  }}
  ifNodeProbe := transform.NewIfNode()
  ifParamsProbe := map[string]interface{}{
    "conditions": map[string]interface{}{
      "combinator": "or",
      "conditions": []interface{}{map[string]interface{}{
        "leftValue":  "={{ $json.data.attributes.route.toLowerCase() }}",
        "operator":   map[string]interface{}{"operation": "equals", "type": "string"},
        "rightValue": "async",
      }},
      "options": map[string]interface{}{"caseSensitive": true, "typeValidation": "strict", "version": 3},
    },
  }
  outProbe, errProbe := ifNodeProbe.Execute([]model.DataItem{m9mHTTPShape}, ifParamsProbe)
  var tagsProbe []interface{}
  for _, it := range outProbe {
    tagsProbe = append(tagsProbe, it.JSON["_ifResult"])
  }
  agentLog("H13", "http_debug_test.go:m9mHTTPShape", "isAsync on current HTTP output shape", map[string]interface{}{
    "err": fmt.Sprintf("%v", errProbe), "tags": tagsProbe,
  })

  // ServiceRoutePlans-shaped item (n8n flattened)
  routeItem := model.DataItem{JSON: map[string]interface{}{
    "data": map[string]interface{}{
      "matched": true,
      "target_rule": "service-rules-order",
      "attributes": map[string]interface{}{"route": "async"},
    },
  }}

  ifNode := transform.NewIfNode()
  ifParams := map[string]interface{}{
    "conditions": map[string]interface{}{
      "combinator": "or",
      "conditions": []interface{}{map[string]interface{}{
        "id": "da4e299b-6a88-4440-b949-66699cac7c39",
        "leftValue": "={{ $json.data.attributes.route.toLowerCase() }}",
        "operator": map[string]interface{}{"operation":"equals","type":"string"},
        "rightValue": "async",
      }},
      "options": map[string]interface{}{"caseSensitive":true,"leftValue":"","typeValidation":"strict","version":3},
    },
  }
  out, err := ifNode.Execute([]model.DataItem{routeItem}, ifParams)
  agentLog("H9", "http_debug_test.go:isAsync", "isAsync IF evaluation", map[string]interface{}{
    "err": fmt.Sprintf("%v", err),
    "outLen": len(out),
    "tags": func() []interface{} {
      var tags []interface{}
      for _, it := range out { tags = append(tags, it.JSON["_ifResult"]) }
      return tags
    }(),
  })

  // Call External Svc-shaped: URL from nested path
  svcItem := model.DataItem{JSON: map[string]interface{}{
    "data": map[string]interface{}{
      "request": map[string]interface{}{
        "target_url": "https://httpbin.org/post",
        "headers": map[string]interface{}{"Content-Type":"application/xml"},
        "body": "<data/>",
      },
    },
  }}
  httpN := httpnode.NewHTTPRequestNode()
  _, herr := httpN.Execute([]model.DataItem{svcItem}, map[string]interface{}{
    "method": "POST",
    "url": "={{ $json.data.request.target_url }}",
    "sendBody": true,
    "contentType": "raw",
    "body": "={{ $json.data.request.body }}",
    "allowInternalRequests": true,
  })
  agentLog("H10", "http_debug_test.go:callExternal", "Call External URL resolve", map[string]interface{}{
    "err": fmt.Sprintf("%v", herr),
  })
}
