package decision

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/tidwall/gjson"
)

type question struct {
	ID      string
	Name    any
	Type    string
	Options []string
}
type Conversion struct {
	From      Dialect
	Questions []question
}
type pair struct {
	key   string
	value any
}
type object []pair

func (o object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(p.key)
		v, e := json.Marshal(p.value)
		if e != nil {
			return nil, e
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
func fields(v gjson.Result, allowed ...string) error {
	if !v.IsObject() {
		return fmt.Errorf("expected an object")
	}
	var err error
	v.ForEach(func(k, _ gjson.Result) bool {
		if !slices.Contains(allowed, k.String()) {
			err = fmt.Errorf("unsupported field %q in decision conversion", k.String())
			return false
		}
		return true
	})
	return err
}
func textField(v gjson.Result, key string) (string, error) {
	x := v.Get(key)
	if x.Type != gjson.String {
		return "", fmt.Errorf("%s must be a string for decision conversion", key)
	}
	return x.String(), nil
}

// ConvertRequest converts the explicitly supported common subset. The context
// retains question order and original names for response reconstruction.
func ConvertRequest(body []byte, from, to Dialect) ([]byte, *Conversion, error) {
	if from == to || from == Unknown || to == Unknown {
		return body, nil, nil
	}
	root := gjson.ParseBytes(body)
	if err := fields(root, "model", "input", "state", "questions"); err != nil {
		return nil, nil, err
	}
	model, err := textField(root, "model")
	if err != nil {
		return nil, nil, err
	}
	sourceInput := "input"
	targetInput := "state"
	if from == SystemOne {
		sourceInput, targetInput = "state", "input"
	}
	input, err := textField(root, sourceInput)
	if err != nil {
		return nil, nil, err
	}
	qs := root.Get("questions")
	if from == OpenAI && !qs.IsArray() || from == SystemOne && !qs.IsObject() {
		return nil, nil, fmt.Errorf("questions have wrong format for %s", from)
	}
	ctx := &Conversion{From: from}
	outputObj := object{}
	outputList := []any{}
	seen := map[string]bool{}
	// Reserve explicit names before assigning IDs to unnamed questions so an
	// internal ID cannot collide with a name that appears later in the array.
	reserved := map[string]bool{}
	if from == OpenAI {
		qs.ForEach(func(_, q gjson.Result) bool {
			if name := q.Get("name"); name.Type == gjson.String && name.String() != "" {
				reserved[name.String()] = true
			}
			return true
		})
	}
	var failure error
	qs.ForEach(func(k, q gjson.Result) bool {
		if len(ctx.Questions) >= 200 {
			failure = fmt.Errorf("too many questions for conversion")
			return false
		}
		if err := fields(q, "type", "name", "instructions", "choices", "levels", "criteria"); err != nil {
			failure = err
			return false
		}
		if from == SystemOne && q.Get("name").Exists() {
			failure = fmt.Errorf("unsupported field name in System One question")
			return false
		}
		typ := q.Get("type").String()
		name := any(nil)
		id := ""
		if from == OpenAI {
			n := q.Get("name")
			if n.Exists() {
				if n.Type != gjson.String {
					failure = fmt.Errorf("question name must be a string")
					return false
				}
				name = n.String()
				id = n.String()
			}
			if id == "" {
				for suffix := len(ctx.Questions); ; suffix++ {
					id = fmt.Sprintf("__question_%d", suffix)
					if !reserved[id] && !seen[id] {
						break
					}
				}
			}
		} else {
			id = k.String()
			name = id
		}
		if seen[id] {
			failure = fmt.Errorf("duplicate question name %q", id)
			return false
		}
		seen[id] = true
		instructions, e := textField(q, "instructions")
		if e != nil {
			failure = e
			return false
		}
		meta := question{ID: id, Name: name, Type: typ}
		dest := map[string]any{"instructions": instructions}
		if from == OpenAI {
			dest["type"] = typ
			if typ == "predicate" {
				dest["type"] = "noul"
			}
		} else {
			dest["type"] = typ
			dest["name"] = id
			if typ == "noul" {
				dest["type"] = "predicate"
			}
		}
		switch typ {
		case "predicate", "noul":
			if (from == OpenAI && typ != "predicate") || (from == SystemOne && typ != "noul") || q.Get("criteria").Exists() || q.Get("choices").Exists() || q.Get("levels").Exists() {
				failure = fmt.Errorf("unsupported predicate fields for conversion")
				return false
			}
		case "choice":
			criteria := object{}
			choices := []any{}
			seenOptions := map[string]bool{}
			if from == OpenAI {
				if q.Get("criteria").Exists() || q.Get("levels").Exists() || !q.Get("choices").IsArray() {
					failure = fmt.Errorf("invalid choice fields")
					return false
				}
				for _, v := range q.Get("choices").Array() {
					if e := fields(v, "value", "description"); e != nil {
						failure = e
						return false
					}
					value, e := textField(v, "value")
					if e != nil {
						failure = e
						return false
					}
					if seenOptions[value] {
						failure = fmt.Errorf("duplicate choice %q", value)
						return false
					}
					seenOptions[value] = true
					desc := ""
					if v.Get("description").Exists() {
						desc, e = textField(v, "description")
						if e != nil {
							failure = e
							return false
						}
					}
					criteria = append(criteria, pair{value, desc})
					meta.Options = append(meta.Options, value)
				}
				dest["criteria"] = criteria
			} else {
				if q.Get("choices").Exists() || q.Get("levels").Exists() || !q.Get("criteria").IsObject() {
					failure = fmt.Errorf("invalid choice fields")
					return false
				}
				q.Get("criteria").ForEach(func(k, v gjson.Result) bool {
					if v.Type != gjson.String {
						failure = fmt.Errorf("choice descriptions must be strings")
						return false
					}
					meta.Options = append(meta.Options, k.String())
					choices = append(choices, map[string]any{"value": k.String(), "description": v.String()})
					return true
				})
				if failure != nil {
					return false
				}
				dest["choices"] = choices
			}
			if len(meta.Options) < 2 || len(meta.Options) > 255 {
				failure = fmt.Errorf("choice requires 2 to 255 options for conversion")
				return false
			}
		case "score":
			levels := []any{}
			if from == OpenAI {
				if q.Get("criteria").Exists() || q.Get("choices").Exists() || !q.Get("levels").IsArray() {
					failure = fmt.Errorf("invalid score fields")
					return false
				}
				for _, v := range q.Get("levels").Array() {
					if e := fields(v, "label"); e != nil {
						failure = e
						return false
					}
					label, e := textField(v, "label")
					if e != nil {
						failure = e
						return false
					}
					levels = append(levels, label)
					meta.Options = append(meta.Options, label)
				}
				dest["criteria"] = levels
			} else {
				if q.Get("choices").Exists() || q.Get("levels").Exists() || !q.Get("criteria").IsArray() {
					failure = fmt.Errorf("invalid score fields")
					return false
				}
				for _, v := range q.Get("criteria").Array() {
					if v.Type != gjson.String {
						failure = fmt.Errorf("score levels must be strings")
						return false
					}
					meta.Options = append(meta.Options, v.String())
					levels = append(levels, map[string]any{"label": v.String()})
				}
				dest["levels"] = levels
			}
			if len(meta.Options) < 2 || len(meta.Options) > 10 {
				failure = fmt.Errorf("score requires 2 to 10 levels for conversion")
				return false
			}
		default:
			failure = fmt.Errorf("unsupported question type %q for decision conversion", typ)
			return false
		}
		ctx.Questions = append(ctx.Questions, meta)
		if from == OpenAI {
			outputObj = append(outputObj, pair{id, dest})
		} else {
			outputList = append(outputList, dest)
		}
		return true
	})
	if failure != nil {
		return nil, nil, failure
	}
	if len(ctx.Questions) == 0 {
		return nil, nil, fmt.Errorf("at least one question is required")
	}
	var questions any = outputObj
	if from == SystemOne {
		questions = outputList
	}
	out, err := json.Marshal(map[string]any{"model": model, targetInput: input, "questions": questions})
	return out, ctx, err
}

// ConvertResponse converts answers, refusing missing or invalid data rather
// than inventing confidence, probability or score values.
func (c *Conversion) ConvertResponse(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, fmt.Errorf("upstream returned invalid JSON")
	}
	root := gjson.ParseBytes(body)
	if root.Get("model").Type != gjson.String {
		return nil, fmt.Errorf("upstream response is missing a valid model")
	}
	answers := root.Get("answers")
	if c.From == OpenAI && !answers.IsObject() || c.From == SystemOne && !answers.IsArray() {
		return nil, fmt.Errorf("upstream answers have incompatible format")
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	// Preserve upstream metadata and normalize the token spellings only when
	// the upstream supplied the corresponding values. Cache details are not
	// synthesized when the provider did not report them.
	if usage := root.Get("usage"); usage.IsObject() {
		var u map[string]json.RawMessage
		if err := json.Unmarshal([]byte(usage.Raw), &u); err != nil {
			return nil, err
		}
		for _, names := range [][]string{{"input_tokens", "inputTokens", "prompt_tokens"}, {"output_tokens", "outputTokens", "completion_tokens"}} {
			if _, ok := u[names[0]]; !ok {
				for _, alias := range names[1:] {
					if v, ok := u[alias]; ok {
						u[names[0]] = v
						break
					}
				}
			}
		}
		if _, ok := u["total_tokens"]; !ok {
			var in, out int64
			if json.Unmarshal(u["input_tokens"], &in) == nil && json.Unmarshal(u["output_tokens"], &out) == nil {
				u["total_tokens"], _ = json.Marshal(in + out)
			}
		}
		raw["usage"], _ = json.Marshal(u)
	}
	delete(raw, "answers")
	resultArray := []any{}
	resultObj := object{}
	list := answers.Array()
	answerMap := answers.Map()
	if c.From == SystemOne && len(list) != len(c.Questions) {
		return nil, fmt.Errorf("upstream answer count mismatch")
	}
	if c.From == OpenAI && len(answerMap) != len(c.Questions) {
		return nil, fmt.Errorf("upstream answer count mismatch")
	}
	for i, q := range c.Questions {
		var a gjson.Result
		if c.From == OpenAI {
			a = answerMap[q.ID]
		} else {
			a = list[i]
			name := a.Get("name")
			if name.Type != gjson.String || name.String() != q.ID {
				return nil, fmt.Errorf("upstream answer name mismatch for %q", q.ID)
			}
		}
		if !a.IsObject() {
			return nil, fmt.Errorf("missing upstream answer for %q", q.ID)
		}
		allowed := []string{"type"}
		if c.From == SystemOne {
			allowed = append(allowed, "name")
		}
		switch q.Type {
		case "predicate":
			allowed = append(allowed, "noul")
		case "noul":
			allowed = append(allowed, "probability")
		case "choice":
			allowed = append(allowed, "choice", "confidence", "probabilities")
		case "score":
			allowed = append(allowed, "score", "confidence", "probabilities")
			if c.From == OpenAI {
				allowed = append(allowed, "legend")
			}
		}
		if err := fields(a, allowed...); err != nil {
			return nil, fmt.Errorf("unsupported upstream answer for %q: %w", q.ID, err)
		}
		typ := a.Get("type").String()
		targetType := q.Type
		if q.Type == "predicate" {
			targetType = "noul"
		}
		if q.Type == "noul" {
			targetType = "predicate"
		}
		if typ != targetType {
			return nil, fmt.Errorf("unexpected upstream answer type %q for %q", typ, q.ID)
		}
		out := map[string]any{"type": q.Type}
		if c.From == OpenAI {
			out["name"] = q.Name
		}
		switch q.Type {
		case "predicate", "noul":
			src, dst := "noul", "probability"
			if c.From == SystemOne {
				src, dst = "probability", "noul"
			}
			p := a.Get(src)
			if p.Type != gjson.Number || p.Float() < 0 || p.Float() > 1 {
				return nil, fmt.Errorf("invalid probability for %q", q.ID)
			}
			out[dst] = p.Float()
		case "choice", "score":
			options := make(map[string]bool, len(q.Options))
			for _, option := range q.Options {
				options[option] = true
			}
			confidence := a.Get("confidence")
			if confidence.Type != gjson.Number || confidence.Float() < 0 || confidence.Float() > 1 {
				return nil, fmt.Errorf("invalid confidence for %q", q.ID)
			}
			out["confidence"] = confidence.Float()
			val := a.Get(q.Type)
			if q.Type == "choice" {
				if val.Type != gjson.String {
					return nil, fmt.Errorf("invalid choice for %q", q.ID)
				}
				if !options[val.String()] {
					return nil, fmt.Errorf("unknown choice for %q", q.ID)
				}
				out["choice"] = val.String()
			} else {
				if val.Type != gjson.Number {
					return nil, fmt.Errorf("invalid score for %q", q.ID)
				}
				out["score"] = val.Float()
			}
			probs := a.Get("probabilities")
			outList := []any{}
			outMap := map[string]float64{}
			if c.From == OpenAI {
				probabilities := probs.Map()
				if !probs.IsObject() || len(probabilities) != len(q.Options) {
					return nil, fmt.Errorf("invalid probabilities for %q", q.ID)
				}
				for n, opt := range q.Options {
					key := opt
					if q.Type == "score" {
						key = strconv.Itoa(n)
						label := a.Get("legend").Get(key)
						if label.Type != gjson.String || label.String() != opt {
							return nil, fmt.Errorf("invalid score legend for %q", q.ID)
						}
					}
					p := probabilities[key]
					if p.Type != gjson.Number || p.Float() < 0 || p.Float() > 1 {
						return nil, fmt.Errorf("invalid probabilities for %q", q.ID)
					}
					if q.Type == "choice" {
						outList = append(outList, map[string]any{"value": opt, "probability": p.Float()})
					} else {
						outList = append(outList, map[string]any{"value": n, "label": opt, "probability": p.Float()})
					}
				}
				out["probabilities"] = outList
			} else {
				if !probs.IsArray() || len(probs.Array()) != len(q.Options) {
					return nil, fmt.Errorf("invalid probabilities for %q", q.ID)
				}
				seen := map[string]bool{}
				for _, p := range probs.Array() {
					probabilityFields := []string{"value", "probability"}
					if q.Type == "score" {
						probabilityFields = append(probabilityFields, "label")
					}
					if err := fields(p, probabilityFields...); err != nil {
						return nil, err
					}
					v := p.Get("value")
					key := ""
					if q.Type == "choice" {
						if v.Type != gjson.String {
							return nil, fmt.Errorf("invalid choice probability")
						}
						key = v.String()
					} else {
						if v.Type != gjson.Number || float64(v.Int()) != v.Float() || v.Int() < 0 || v.Int() >= int64(len(q.Options)) {
							return nil, fmt.Errorf("invalid score probability")
						}
						key = strconv.FormatInt(v.Int(), 10)
						label := p.Get("label")
						if label.Type != gjson.String || label.String() != q.Options[v.Int()] {
							return nil, fmt.Errorf("invalid score label")
						}
					}
					valid := q.Type == "score" || options[key]
					prob := p.Get("probability")
					if !valid || seen[key] || prob.Type != gjson.Number || prob.Float() < 0 || prob.Float() > 1 {
						return nil, fmt.Errorf("invalid probabilities for %q", q.ID)
					}
					seen[key] = true
					outMap[key] = prob.Float()
				}
				out["probabilities"] = outMap
				if q.Type == "score" {
					legend := map[string]string{}
					for n, v := range q.Options {
						legend[strconv.Itoa(n)] = v
					}
					out["legend"] = legend
				}
			}
		}
		if c.From == OpenAI {
			resultArray = append(resultArray, out)
		} else {
			resultObj = append(resultObj, pair{q.ID, out})
		}
	}
	var result any = resultArray
	if c.From == SystemOne {
		result = resultObj
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	raw["answers"] = encoded
	return json.Marshal(raw)
}
