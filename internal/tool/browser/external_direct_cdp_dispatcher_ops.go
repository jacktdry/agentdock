package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

type directTargetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
}

func (d *directCDPDispatcher) targetInfoLocked(ctx context.Context) (directTargetInfo, error) {
	var result struct {
		TargetInfo directTargetInfo `json:"targetInfo"`
	}
	if d.targetID == "" {
		return directTargetInfo{}, errDirectCDPAction
	}
	if err := d.callLocked(ctx, "Target.getTargetInfo", "", struct {
		TargetID string `json:"targetId"`
	}{d.targetID}, &result); err != nil {
		return directTargetInfo{}, err
	}
	return result.TargetInfo, nil
}
func (d *directCDPDispatcher) verifyTargetLocked(ctx context.Context) error {
	info, err := d.targetInfoLocked(ctx)
	if err != nil || info.TargetID != d.targetID || info.Type != "page" || d.sessionID == "" {
		d.Close()
		return errDirectCDPRetired
	}
	return nil
}
func (d *directCDPDispatcher) clearUIDsLocked() { clear(d.uidToBackend) }

func directCDPTextResult(text string) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "structuredContent": map[string]any{"message": text}}
}
func directCDPJSONResult(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	return directCDPTextResult("```json\n" + string(raw) + "\n```")
}
func (d *directCDPDispatcher) EvaluateScript(ctx context.Context, function string) (map[string]any, error) {
	function = strings.TrimSpace(function)
	if function == "" || len(function) > 1<<20 {
		return nil, errDirectCDPAction
	}
	raw, err := d.Evaluate(ctx, "("+function+")()")
	if err != nil {
		return nil, err
	}
	return directCDPJSONResult(raw), nil
}

type directAXValue struct {
	Value any `json:"value"`
}
type directAXNode struct {
	BackendDOMNodeID int64         `json:"backendDOMNodeId"`
	Ignored          bool          `json:"ignored"`
	Role             directAXValue `json:"role"`
	Name             directAXValue `json:"name"`
	Value            directAXValue `json:"value"`
}

func directAXString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

func (d *directCDPDispatcher) Snapshot(ctx context.Context, verbose bool) (map[string]any, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := d.verifyTargetLocked(ctx); err != nil {
		return nil, err
	}
	var result struct {
		Nodes []directAXNode `json:"nodes"`
	}
	if err := d.callLocked(ctx, "Accessibility.getFullAXTree", d.sessionID, map[string]any{}, &result); err != nil {
		return nil, err
	}
	d.snapshotSeq++
	clear(d.uidToBackend)
	var b strings.Builder
	written := 0
	for _, node := range result.Nodes {
		if node.Ignored && !verbose {
			continue
		}
		role, name, value := directAXString(node.Role.Value), directAXString(node.Name.Value), directAXString(node.Value.Value)
		if !verbose && role == "" && name == "" && value == "" {
			continue
		}
		uid := ""
		if node.BackendDOMNodeID > 0 {
			uid = "d" + strconv.FormatUint(d.snapshotSeq, 10) + "_" + strconv.Itoa(written)
			d.uidToBackend[uid] = node.BackendDOMNodeID
		}
		if uid != "" {
			b.WriteString("uid=")
			b.WriteString(uid)
			b.WriteByte(' ')
		}
		if role != "" {
			b.WriteString("role=")
			b.WriteString(strconv.Quote(role))
			b.WriteByte(' ')
		}
		if name != "" {
			b.WriteString("name=")
			b.WriteString(strconv.Quote(name))
			b.WriteByte(' ')
		}
		if value != "" {
			b.WriteString("value=")
			b.WriteString(strconv.Quote(value))
		}
		b.WriteByte('\n')
		written++
		if written >= 500 || b.Len() >= 50000 {
			break
		}
	}
	text := strings.TrimSpace(b.String())
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "structuredContent": map[string]any{"message": text, "snapshot_generation": d.snapshotSeq}}, nil
}

func (d *directCDPDispatcher) resolveUIDLocked(ctx context.Context, uid string) (string, error) {
	backend, ok := d.uidToBackend[uid]
	if !ok || backend <= 0 {
		return "", errDirectCDPAction
	}
	var result struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := d.callLocked(ctx, "DOM.resolveNode", d.sessionID, struct {
		BackendNodeID int64 `json:"backendNodeId"`
	}{backend}, &result); err != nil || result.Object.ObjectID == "" {
		return "", errDirectCDPAction
	}
	return result.Object.ObjectID, nil
}
func (d *directCDPDispatcher) releaseObjectLocked(ctx context.Context, id string) error {
	var result struct{}
	return d.callLocked(ctx, "Runtime.releaseObject", d.sessionID, struct {
		ObjectID string `json:"objectId"`
	}{id}, &result)
}
func (d *directCDPDispatcher) callObjectLocked(ctx context.Context, id, fn string, args []map[string]any) (json.RawMessage, error) {
	var result struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	err := d.callLocked(ctx, "Runtime.callFunctionOn", d.sessionID, struct {
		FunctionDeclaration string           `json:"functionDeclaration"`
		ObjectID            string           `json:"objectId"`
		Arguments           []map[string]any `json:"arguments,omitempty"`
		ReturnByValue       bool             `json:"returnByValue"`
		AwaitPromise        bool             `json:"awaitPromise"`
		UserGesture         bool             `json:"userGesture"`
	}{fn, id, args, true, true, false}, &result)
	if err != nil {
		return nil, err
	}
	if len(result.ExceptionDetails) != 0 && string(result.ExceptionDetails) != "null" {
		return nil, errDirectCDPAction
	}
	return result.Result.Value, nil
}

func (d *directCDPDispatcher) Click(ctx context.Context, uid string) (map[string]any, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err = d.verifyTargetLocked(ctx); err != nil {
		return nil, err
	}
	obj, err := d.resolveUIDLocked(ctx, uid)
	if err != nil {
		return nil, err
	}
	_, err = d.callObjectLocked(ctx, obj, "function(){this.click();return true;}", nil)
	rel := d.releaseObjectLocked(ctx, obj)
	if err == nil {
		err = rel
	}
	if err != nil {
		return nil, err
	}
	return directCDPTextResult("clicked " + uid), nil
}
func (d *directCDPDispatcher) Fill(ctx context.Context, uid, value string) (map[string]any, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err = d.verifyTargetLocked(ctx); err != nil {
		return nil, err
	}
	obj, err := d.resolveUIDLocked(ctx, uid)
	if err != nil {
		return nil, err
	}
	fn := `function(value){if(this.disabled||this.readOnly)return false;if('value' in this)this.value=value;else if(this.isContentEditable)this.textContent=value;else return false;this.dispatchEvent(new Event('input',{bubbles:true}));this.dispatchEvent(new Event('change',{bubbles:true}));return true;}`
	raw, err := d.callObjectLocked(ctx, obj, fn, []map[string]any{{"value": value}})
	rel := d.releaseObjectLocked(ctx, obj)
	if err == nil {
		err = rel
	}
	if err != nil || string(raw) != "true" {
		return nil, errDirectCDPAction
	}
	return directCDPTextResult("filled " + uid), nil
}
func directSingleKey(key string) bool {
	return key != "" && key == strings.TrimSpace(key) && len(key) <= 32 && !strings.ContainsAny(key, "+\n\r\t")
}
func (d *directCDPDispatcher) PressKey(ctx context.Context, key string) (map[string]any, error) {
	if !directSingleKey(key) {
		return nil, errDirectCDPAction
	}
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err = d.verifyTargetLocked(ctx); err != nil {
		return nil, err
	}
	for _, kind := range []string{"keyDown", "keyUp"} {
		var result struct{}
		if err = d.callLocked(ctx, "Input.dispatchKeyEvent", d.sessionID, struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		}{kind, key}, &result); err != nil {
			return nil, err
		}
	}
	return directCDPTextResult("pressed " + key), nil
}
func (d *directCDPDispatcher) Screenshot(ctx context.Context, fullPage bool) (map[string]any, error) {
	ctx, unlock, err := d.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err = d.verifyTargetLocked(ctx); err != nil {
		return nil, err
	}
	var result struct {
		Data string `json:"data"`
	}
	if err = d.callLocked(ctx, "Page.captureScreenshot", d.sessionID, struct {
		Format                string `json:"format"`
		FromSurface           bool   `json:"fromSurface"`
		CaptureBeyondViewport bool   `json:"captureBeyondViewport"`
	}{"png", true, fullPage}, &result); err != nil {
		return nil, err
	}
	if result.Data == "" {
		return nil, errDirectCDPAction
	}
	if _, err = base64.StdEncoding.DecodeString(result.Data); err != nil {
		return nil, errDirectCDPAction
	}
	return map[string]any{"content": []any{map[string]any{"type": "image", "data": result.Data, "mimeType": "image/png"}}, "structuredContent": map[string]any{"mime_type": "image/png", "full_page": fullPage}}, nil
}
