package importer

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

type Request struct {
	Method          string
	Target          string
	Path            string
	Headers         map[string]string
	Body            string
	FollowRedirects bool
	User            string
	Password        string
}

type Import struct {
	YAML     string
	Warnings []string
}

func FromCurl(command string) (Import, error) {
	fields, err := split(command)
	if err != nil {
		return Import{}, err
	}
	request, err := interpretar(fields)
	if err != nil {
		return Import{}, err
	}
	return build(request), nil
}

// Quotes and line-continuation backslashes come glued to what the person
// copied from the browser; splitting on plain spaces would break every JSON
// body.
func split(command string) ([]string, error) {
	var fields []string
	var current strings.Builder
	has := false
	quote := rune(0)

	runas := []rune(command)
	for index := 0; index < len(runas); index++ {
		char := runas[index]
		switch {
		case char == '\\' && quote != '\'':
			if index+1 < len(runas) {
				next := runas[index+1]
				if next == '\n' {
					index++
					continue
				}
				if quote == 0 {
					current.WriteRune(next)
					has = true
					index++
					continue
				}
				if next == '"' || next == '\\' {
					current.WriteRune(next)
					has = true
					index++
					continue
				}
			}
			current.WriteRune(char)
			has = true
		case quote != 0:
			if char == quote {
				quote = 0
				continue
			}
			current.WriteRune(char)
			has = true
		case char == '\'' || char == '"':
			quote = char
			has = true
		case unicode.IsSpace(char):
			if has {
				fields = append(fields, current.String())
				current.Reset()
				has = false
			}
		default:
			current.WriteRune(char)
			has = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("o comando tem aspas abertas que nunca fecham; cole o curl inteiro, inclusive a ultima linha")
	}
	if has {
		fields = append(fields, current.String())
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("nao recebi nenhum comando; use:\n  braunrate import curl \"curl -X POST https://exemplo/pedidos -d '{}'\"\nou passe o comando pela entrada padrao")
	}
	return fields, nil
}

func interpretar(fields []string) (Request, error) {
	request := Request{Headers: map[string]string{}}
	if fields[0] == "curl" {
		fields = fields[1:]
	}

	next := func(index *int, flag string) (string, error) {
		if *index+1 >= len(fields) {
			return "", fmt.Errorf("a opcao %s ficou sem valor no fim do comando", flag)
		}
		*index++
		return fields[*index], nil
	}

	var address string
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		name, attachedValue, attached := strings.Cut(field, "=")

		switch {
		case field == "-X" || field == "--request":
			value, err := next(&index, field)
			if err != nil {
				return request, err
			}
			request.Method = strings.ToUpper(value)
		case field == "-H" || field == "--header":
			value, err := next(&index, field)
			if err != nil {
				return request, err
			}
			key, content, has := strings.Cut(value, ":")
			if !has {
				return request, fmt.Errorf("o cabecalho %q nao tem dois-pontos; a forma e -H \"Nome: valor\"", value)
			}
			request.Headers[strings.TrimSpace(key)] = strings.TrimSpace(content)
		case field == "-d" || field == "--data" || field == "--data-raw" || field == "--data-binary" || field == "--data-ascii":
			value, err := next(&index, field)
			if err != nil {
				return request, err
			}
			request.Body = value
		case attached && (name == "--data" || name == "--data-raw" || name == "--data-binary"):
			request.Body = attachedValue
		case field == "-u" || field == "--user":
			value, err := next(&index, field)
			if err != nil {
				return request, err
			}
			request.User, request.Password, _ = strings.Cut(value, ":")
		case field == "--url":
			value, err := next(&index, field)
			if err != nil {
				return request, err
			}
			address = value
		case field == "-L" || field == "--location":
			request.FollowRedirects = true
		case field == "-o" || field == "--output" || field == "-w" || field == "--write-out" || field == "-A" || field == "--user-agent" || field == "-b" || field == "--cookie" || field == "-e" || field == "--referer":
			value, err := next(&index, field)
			if err != nil {
				return request, err
			}
			if field == "-A" || field == "--user-agent" {
				request.Headers["User-Agent"] = value
			}
			if field == "-b" || field == "--cookie" {
				request.Headers["Cookie"] = value
			}
		case strings.HasPrefix(field, "-"):
			continue
		default:
			if address == "" {
				address = field
			}
		}
	}

	if address == "" {
		return request, fmt.Errorf("nao achei a URL no comando; o curl precisa ter o endereco, como em:\n  curl https://exemplo/pedidos")
	}
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	parts, err := url.Parse(address)
	if err != nil {
		return request, fmt.Errorf("nao consegui entender a URL %q: %v", address, err)
	}

	request.Target = parts.Scheme + "://" + parts.Host
	request.Path = parts.RequestURI()
	if request.Method == "" {
		request.Method = "GET"
		if request.Body != "" {
			request.Method = "POST"
		}
	}
	return request, nil
}

func build(request Request) Import {
	script := Script{
		Name:   scenarioName(request),
		Target: request.Target,
		Steps: []ImportedStep{{
			Name:            stepName(request),
			Method:          request.Method,
			Path:            request.Path,
			Headers:         request.Headers,
			Body:            request.Body,
			FollowRedirects: request.FollowRedirects,
		}},
	}

	if request.User != "" {
		script.Warnings = append(script.Warnings,
			fmt.Sprintf("o comando usava -u %s:...; declare isso no bloco 'autenticacao' com tipo: basica, e deixe a senha em variavel de ambiente", request.User))
	}
	if strings.Contains(request.Path, "?") || hasIdentifier(request.Path) {
		script.Warnings = append(script.Warnings,
			"o caminho tem valor fixo: com um valor so, o alvo responde de cache e o numero fica otimista. Troque por ${dados.coluna} e declare um bloco 'dados'")
	}
	return RenderYAML(script)
}

func scenarioName(request Request) string {
	return "Importado de curl " + strings.ToUpper(request.Method) + " " + Resource(request.Path)
}

func stepName(request Request) string {
	return strings.ToLower(request.Method) + " " + Resource(request.Path)
}
