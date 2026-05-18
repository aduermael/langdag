package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

func strictJSONUnmarshal(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON")
		}
		return err
	}
	return nil
}

func NormalizeCatalogV1(catalog *CatalogV1) {
	if catalog == nil {
		return
	}
	for _, provider := range catalog.Providers {
		if provider != nil {
			sort.Strings(provider.Aliases)
		}
	}
	for _, model := range catalog.Models {
		if model != nil {
			sort.Strings(model.Aliases)
		}
	}
	sort.SliceStable(catalog.Offerings, func(i, j int) bool {
		if catalog.Offerings[i].CanonicalModelID != catalog.Offerings[j].CanonicalModelID {
			return catalog.Offerings[i].CanonicalModelID < catalog.Offerings[j].CanonicalModelID
		}
		return catalog.Offerings[i].ID < catalog.Offerings[j].ID
	})
	sort.SliceStable(catalog.OfferingTemplates, func(i, j int) bool {
		if catalog.OfferingTemplates[i].CanonicalModelID != catalog.OfferingTemplates[j].CanonicalModelID {
			return catalog.OfferingTemplates[i].CanonicalModelID < catalog.OfferingTemplates[j].CanonicalModelID
		}
		return catalog.OfferingTemplates[i].ID < catalog.OfferingTemplates[j].ID
	})
	for i := range catalog.Offerings {
		normalizePricingV1(&catalog.Offerings[i].Pricing)
	}
	for i := range catalog.OfferingTemplates {
		normalizePricingV1(&catalog.OfferingTemplates[i].Pricing)
	}
}

func normalizePricingV1(pricing *PricingV1) {
	if pricing == nil {
		return
	}
	sort.Strings(pricing.MissingDimensions)
	sort.Strings(pricing.Notes)
}
