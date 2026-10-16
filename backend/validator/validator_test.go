package validator

import (
    "testing"
)

func TestSanitizeStringEscapesHTML(t *testing.T) {
    input := " <script>alert(1)</script>  "
    output := SanitizeString(input)

    if output != "&lt;script&gt;alert(1)&lt;/script&gt;" {
        t.Fatalf("expected escaped output, got %q", output)
    }
}

func TestValidateTokenizeAssetRequest(t *testing.T) {
    if err := Init(); err != nil {
        t.Fatalf("failed to initialize validator: %v", err)
    }

    req := TokenizeAssetRequest{
        IssuerAccount: "GCNIPSNVY5VAEAP3YNQWF7ZARHP5FZRE33XJO5CV7RET7XCZX3JGVUPE",
        Name:          "Real Asset",
        Symbol:        "RWA1",
        AssetType:     "real_estate",
        TotalSupply:   1000,
    }

    if err := ValidateStruct(&req); err != nil {
        t.Fatalf("expected request to validate, got %v", err)
    }
}

func TestValidateTokenizeAssetRequestRejectsHtml(t *testing.T) {
    if err := Init(); err != nil {
        t.Fatalf("failed to initialize validator: %v", err)
    }

    req := TokenizeAssetRequest{
        IssuerAccount: "GCNIPSNVY5VAEAP3YNQWF7ZARHP5FZRE33XJO5CV7RET7XCZX3JGVUPE",
        Name:          "Real <b>Asset</b>",
        Symbol:        "RWA1",
        AssetType:     "real_estate",
        TotalSupply:   1000,
    }

    if err := ValidateStruct(&req); err == nil {
        t.Fatal("expected validation error for HTML in name")
    }
}
