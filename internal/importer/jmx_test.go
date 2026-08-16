package importer_test

import (
	"strings"
	"testing"

	"github.com/Diegobraun/braunrate/internal/importer"
	_ "github.com/Diegobraun/braunrate/internal/protocol/http"
	"github.com/Diegobraun/braunrate/internal/scenario"
)

const jmeterPlan = `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2" properties="5.0" jmeter="5.6.3">
  <hashTree>
    <TestPlan guiclass="TestPlanGui" testclass="TestPlan" testname="Cobranca" enabled="true"/>
    <hashTree>
      <CSVDataSet guiclass="TestBeanGUI" testclass="CSVDataSet" testname="assinantes" enabled="true">
        <stringProp name="filename">dados/assinantes.csv</stringProp>
        <boolProp name="recycle">true</boolProp>
      </CSVDataSet>
      <hashTree/>
      <ThreadGroup guiclass="ThreadGroupGui" testclass="ThreadGroup" testname="Usuarios" enabled="true">
        <stringProp name="ThreadGroup.num_threads">50</stringProp>
        <stringProp name="ThreadGroup.ramp_time">30</stringProp>
        <stringProp name="ThreadGroup.duration">300</stringProp>
      </ThreadGroup>
      <hashTree>
        <HeaderManager guiclass="HeaderPanel" testclass="HeaderManager" testname="Cabecalhos" enabled="true">
          <collectionProp name="HeaderManager.headers">
            <elementProp name="" elementType="Header">
              <stringProp name="Header.name">Authorization</stringProp>
              <stringProp name="Header.value">Bearer token-secreto-de-verdade</stringProp>
            </elementProp>
            <elementProp name="" elementType="Header">
              <stringProp name="Header.name">X-Inquilino</stringProp>
              <stringProp name="Header.value">acme</stringProp>
            </elementProp>
          </collectionProp>
        </HeaderManager>
        <hashTree/>
        <HTTPSamplerProxy guiclass="HttpTestSampleGui" testclass="HTTPSamplerProxy" testname="consultar pedido" enabled="true">
          <stringProp name="HTTPSampler.domain">api.exemplo.com</stringProp>
          <stringProp name="HTTPSampler.port">443</stringProp>
          <stringProp name="HTTPSampler.protocol">https</stringProp>
          <stringProp name="HTTPSampler.path">/v1/pedidos/9912</stringProp>
          <stringProp name="HTTPSampler.method">GET</stringProp>
        </HTTPSamplerProxy>
        <hashTree>
          <JSONPostProcessor guiclass="JSONPostProcessorGui" testclass="JSONPostProcessor" testname="pegar fatura" enabled="true">
            <stringProp name="JSONPostProcessor.referenceNames">faturaId</stringProp>
            <stringProp name="JSONPostProcessor.jsonPathExprs">$.ultimaFatura.id</stringProp>
          </JSONPostProcessor>
          <hashTree/>
          <ResponseAssertion guiclass="AssertionGui" testclass="ResponseAssertion" testname="status 200" enabled="true"/>
          <hashTree/>
        </hashTree>
        <HTTPSamplerProxy guiclass="HttpTestSampleGui" testclass="HTTPSamplerProxy" testname="pagar fatura" enabled="true">
          <stringProp name="HTTPSampler.domain">api.exemplo.com</stringProp>
          <stringProp name="HTTPSampler.protocol">https</stringProp>
          <stringProp name="HTTPSampler.path">/v1/faturas/9912/pagamento</stringProp>
          <stringProp name="HTTPSampler.method">POST</stringProp>
          <elementProp name="HTTPsampler.Arguments" elementType="Arguments">
            <collectionProp name="Arguments.arguments">
              <elementProp name="" elementType="HTTPArgument">
                <stringProp name="Argument.value">{"valor": 199.9}</stringProp>
              </elementProp>
            </collectionProp>
          </elementProp>
        </HTTPSamplerProxy>
        <hashTree/>
        <BeanShellPreProcessor guiclass="TestBeanGUI" testclass="BeanShellPreProcessor" testname="script" enabled="true"/>
        <hashTree/>
      </hashTree>
    </hashTree>
  </hashTree>
</jmeterTestPlan>
`

func TestImportsJMXIntoParsableScenario(t *testing.T) {
	importResult, err := importer.FromJMX([]byte(jmeterPlan))
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}

	c, err := scenario.Parse([]byte(importResult.YAML))
	if err != nil {
		t.Fatalf("gerei um cenario que o parser recusa:\n%v\n%s", err, importResult.YAML)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("cenario invalido: %v\n%s", err, importResult.YAML)
	}
	if len(c.Steps) != 2 {
		t.Fatalf("esperava 2 passos, veio %d:\n%s", len(c.Steps), importResult.YAML)
	}
	if c.Target != "https://api.exemplo.com" {
		t.Errorf("alvo = %q", c.Target)
	}
	if c.Steps[0].Name != "consultar pedido" || c.Steps[1].Name != "pagar fatura" {
		t.Errorf("nomes dos passos: %q e %q", c.Steps[0].Name, c.Steps[1].Name)
	}
	if len(c.Data) != 1 || c.Data[0].File != "dados/assinantes.csv" {
		t.Errorf("bloco de dados nao veio do CSVDataSet: %+v", c.Data)
	}
}

func TestJMXTokenNeverReachesFile(t *testing.T) {
	importResult, err := importer.FromJMX([]byte(jmeterPlan))
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if strings.Contains(importResult.YAML, "token-secreto-de-verdade") {
		t.Fatalf("o token do .jmx foi escrito no cenario:\n%s", importResult.YAML)
	}
	if !strings.Contains(importResult.YAML, "${token}") {
		t.Errorf("o cabecalho de credencial devia virar variavel:\n%s", importResult.YAML)
	}
}

// Silently turning threads into a rate would import coordinated omission along
// with the scenario: the person would believe they measured 50/s.
func TestWarnsThreadsAreNotArrivalRate(t *testing.T) {
	importResult, err := importer.FromJMX([]byte(jmeterPlan))
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if !containsFragment(importResult.Warnings, "50 threads") || !containsFragment(importResult.Warnings, "nao vira taxa de chegada") {
		t.Errorf("faltou o aviso sobre thread nao virar taxa: %v", importResult.Warnings)
	}
}

func TestDeclaresWhatWasNotTranslated(t *testing.T) {
	importResult, err := importer.FromJMX([]byte(jmeterPlan))
	if err != nil {
		t.Fatalf("nao importou: %v", err)
	}
	if !containsFragment(importResult.Warnings, "BeanShellPreProcessor") {
		t.Errorf("elemento ignorado precisa ser declarado: %v", importResult.Warnings)
	}
	if !containsFragment(importResult.Warnings, "faturaId") {
		t.Errorf("a correlacao do .jmx precisa virar instrucao de captura: %v", importResult.Warnings)
	}
}

func TestJMXWithoutHTTPRequestExplainsImporterCoverage(t *testing.T) {
	_, err := importer.FromJMX([]byte(`<?xml version="1.0"?><jmeterTestPlan><hashTree><TestPlan testname="vazio"/></hashTree></jmeterTestPlan>`))
	if err == nil {
		t.Fatal("plano sem requisicao precisa falhar")
	}
	if !strings.Contains(err.Error(), "HTTPSamplerProxy") {
		t.Errorf("o erro precisa dizer o que o importador cobre: %v", err)
	}
}

func containsFragment(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}
