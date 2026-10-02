package factory

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// FetchJSON never includes request URLs, codes or provider bodies in errors.
// Reject redirects so authentication headers/body cannot cross endpoints.
func FetchJSON(req *http.Request, target any) error {
    client := &http.Client{Timeout:10*time.Second, CheckRedirect:func(*http.Request,[]*http.Request) error { return http.ErrUseLastResponse }}
    response, err := client.Do(req)
    if err != nil { return fmt.Errorf("provider request failed") }
    defer response.Body.Close()
    if response.StatusCode<200 || response.StatusCode>=300 { return fmt.Errorf("provider returned HTTP %d",response.StatusCode) }
    body, err := io.ReadAll(io.LimitReader(response.Body,(64<<10)+1))
    if err != nil || len(body)>64<<10 { return fmt.Errorf("provider response unreadable or too large") }
    if err := json.Unmarshal(body,target); err != nil { return fmt.Errorf("invalid provider JSON response") }
    return nil
}
