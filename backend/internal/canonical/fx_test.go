package canonical

import "testing"

func TestDefaultConverterIdentityOnly(t *testing.T) {
	converter := DefaultConverter{}
	if amount, err := converter.ConvertMoney(1250, "EUR", "EUR"); err != nil || amount != 1250 {
		t.Fatalf("identity money = %d %v", amount, err)
	}
	if _, err := converter.ConvertMoney(1250, "EUR", "USD"); err == nil {
		t.Fatal("cross-currency conversion should fail closed")
	}
	if quantity, err := converter.ConvertQuantity("2.250000", "piece", "piece"); err != nil || quantity != "2.250000" {
		t.Fatalf("identity quantity = %q %v", quantity, err)
	}
	if _, err := converter.ConvertQuantity("2.250000", "piece", "kg"); err == nil {
		t.Fatal("cross-unit conversion should fail closed")
	}
}
