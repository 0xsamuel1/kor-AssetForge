'use client'

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { assetApi, FilterFacet } from "@/lib/asset-api";
import { cn, formatCurrency } from "@/lib/utils";
import { ChevronDown, ChevronUp, X, SlidersHorizontal, RotateCcw } from "lucide-react";

interface FilterPanelProps {
  filters: Record<string, string>;
  onFiltersChange: (filters: Record<string, string>) => void;
  className?: string;
}

const FILTER_LABELS: Record<string, string> = {
  type: "Asset Type",
  location: "Location",
  status: "Status",
  price: "Price Range",
};

export function FilterPanel({ filters, onFiltersChange, className }: FilterPanelProps) {
  const [facets, setFacets] = useState<FilterFacet[]>([]);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [priceMin, setPriceMin] = useState("");
  const [priceMax, setPriceMax] = useState("");

  useEffect(() => {
    const load = async () => {
      try {
        const data = await assetApi.getFilterFacets();
        setFacets(data);
      } catch {
        console.error("Failed to load facets");
      }
    };
    load();
  }, []);

  useEffect(() => {
    setPriceMin(filters.price_min || "");
    setPriceMax(filters.price_max || "");
  }, [filters.price_min, filters.price_max]);

  const toggleFilter = (facetName: string, value: string) => {
    const key = facetName.toLowerCase();
    const current = filters[key];
    if (current === value) {
      const next = { ...filters };
      delete next[key];
      onFiltersChange(next);
    } else {
      onFiltersChange({ ...filters, [key]: value });
    }
  };

  const applyPriceFilter = () => {
    const next = { ...filters };
    if (priceMin) next.price_min = priceMin;
    else delete next.price_min;
    if (priceMax) next.price_max = priceMax;
    else delete next.price_max;
    onFiltersChange(next);
  };

  const clearAll = () => {
    onFiltersChange({});
    setPriceMin("");
    setPriceMax("");
  };

  const activeCount = Object.keys(filters).length;

  return (
    <div className={cn("space-y-4", className)}>
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <SlidersHorizontal className="h-4 w-4" aria-hidden="true" />
          <span className="font-semibold text-sm">Filters</span>
          {activeCount > 0 && (
            <Badge variant="secondary" className="text-xs">{activeCount}</Badge>
          )}
        </div>
        {activeCount > 0 && (
          <Button variant="ghost" size="sm" onClick={clearAll} className="h-7 text-xs">
            <RotateCcw className="h-3 w-3 mr-1" aria-hidden="true" /> Clear
          </Button>
        )}
      </div>

      {/* Active Filter Chips */}
      {activeCount > 0 && (
        <div className="flex flex-wrap gap-1">
          {Object.entries(filters).map(([key, value]) => (
            <Badge key={key} variant="secondary" className="gap-1 pr-1">
              <span className="text-xs">{FILTER_LABELS[key] || key}: {value}</span>
              <button
                type="button"
                aria-label={`Remove ${FILTER_LABELS[key] || key} filter`}
                onClick={() => {
                  const next = { ...filters };
                  delete next[key];
                  onFiltersChange(next);
                }}
                className="ml-1 hover:bg-muted rounded-full p-0.5"
              >
                <X className="h-3 w-3" aria-hidden="true" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      {/* Price Range Filter */}
      <div>
        <button
          type="button"
          aria-expanded={!!expanded.price}
          aria-controls="filter-section-price"
          className="flex items-center justify-between w-full text-sm font-medium mb-2"
          onClick={() => setExpanded((e) => ({ ...e, price: !e.price }))}
        >
          Price Range
          {expanded.price ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
        </button>
        {expanded.price && (
          <div id="filter-section-price" className="space-y-2 pl-1">
            <div className="flex gap-2">
              <Input
                type="number"
                placeholder="Min"
                aria-label="Minimum price"
                value={priceMin}
                onChange={(e) => setPriceMin(e.target.value)}
                className="h-8 text-sm"
              />
              <Input
                type="number"
                placeholder="Max"
                aria-label="Maximum price"
                value={priceMax}
                onChange={(e) => setPriceMax(e.target.value)}
                className="h-8 text-sm"
              />
            </div>
            <Button size="sm" className="w-full h-7 text-xs" onClick={applyPriceFilter}>
              Apply
            </Button>
          </div>
        )}
      </div>

      {/* Facet Filters */}
      {facets.map((facet) => {
        const key = facet.name.toLowerCase();
        const isExpanded = expanded[facet.name] !== false;
        return (
          <div key={facet.name}>
            <button
              type="button"
              aria-expanded={isExpanded}
              aria-controls={`filter-section-${key}`}
              className="flex items-center justify-between w-full text-sm font-medium mb-2"
              onClick={() => setExpanded((e) => ({ ...e, [facet.name]: !isExpanded }))}
            >
              {facet.name}
              {isExpanded ? <ChevronUp className="h-4 w-4" aria-hidden="true" /> : <ChevronDown className="h-4 w-4" aria-hidden="true" />}
            </button>
            {isExpanded && (
              <div id={`filter-section-${key}`} className="space-y-1 pl-1">
                {facet.values.map((v) => (
                  <button
                    key={v.value}
                    type="button"
                    aria-pressed={filters[key] === v.value}
                    onClick={() => toggleFilter(facet.name, v.value)}
                    className={cn(
                      "flex items-center justify-between w-full text-sm px-2 py-1.5 rounded-md transition-colors",
                      filters[key] === v.value
                        ? "bg-primary/10 text-primary font-medium"
                        : "hover:bg-muted text-muted-foreground"
                    )}
                  >
                    <span>{v.label}</span>
                    <Badge variant="secondary" className="text-xs px-1.5 py-0 h-5">
                      {v.count}
                    </Badge>
                  </button>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
