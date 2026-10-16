'use client'

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { assetApi, FilterFacet } from "@/lib/asset-api";
import { cn } from "@/lib/utils";
import { RotateCcw, SlidersHorizontal, X } from "lucide-react";

interface SearchFiltersProps {
  filters: Record<string, string>;
  onFiltersChange: (filters: Record<string, string>) => void;
  className?: string;
}

const MULTI_VALUE_FILTERS = new Set(["asset_type", "category", "tag", "location"]);

export function SearchFilters({ filters, onFiltersChange, className }: SearchFiltersProps) {
  const [facets, setFacets] = useState<FilterFacet[]>([]);
  const [minPrice, setMinPrice] = useState(filters.min_price || "");
  const [maxPrice, setMaxPrice] = useState(filters.max_price || "");

  useEffect(() => {
    assetApi.getFilterFacets().then(setFacets).catch(() => setFacets([]));
  }, []);

  const toggleValue = (key: string, value: string) => {
    const selected = new Set((filters[key] || "").split(",").filter(Boolean));
    if (MULTI_VALUE_FILTERS.has(key)) {
      if (selected.has(value)) selected.delete(value); else selected.add(value);
    } else {
      selected.clear();
      if (filters[key] !== value) selected.add(value);
    }
    const next = { ...filters };
    if (selected.size) next[key] = [...selected].join(","); else delete next[key];
    onFiltersChange(next);
  };

  const setFilter = (key: string, value: string) => {
    const next = { ...filters };
    if (value) next[key] = value; else delete next[key];
    onFiltersChange(next);
  };

  const applyPrice = () => {
    const next = { ...filters };
    if (minPrice) next.min_price = minPrice; else delete next.min_price;
    if (maxPrice) next.max_price = maxPrice; else delete next.max_price;
    onFiltersChange(next);
  };

  const clearAll = () => {
    setMinPrice("");
    setMaxPrice("");
    onFiltersChange({});
  };

  return (
    <section aria-label="Search filters" className={cn("space-y-5", className)}>
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 font-semibold"><SlidersHorizontal className="h-4 w-4" /> Filters <Badge variant="secondary">{Object.keys(filters).length}</Badge></div>
        {Object.keys(filters).length > 0 && <Button variant="ghost" size="sm" onClick={clearAll}><RotateCcw className="mr-1 h-3 w-3" />Clear</Button>}
      </div>

      <div className="flex flex-wrap gap-1">
        {Object.entries(filters).flatMap(([key, value]) => value.split(",").map((item) => (
          <Badge key={`${key}-${item}`} variant="secondary" className="gap-1">
            {key.replaceAll("_", " ")}: {item}
            <button type="button" aria-label={`Remove ${item}`} onClick={() => toggleValue(key, item)}><X className="h-3 w-3" /></button>
          </Badge>
        )))}
      </div>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">Price range</legend>
        <div className="flex gap-2">
          <Input type="number" min="0" aria-label="Minimum price" placeholder="Min" value={minPrice} onChange={(event) => setMinPrice(event.target.value)} />
          <Input type="number" min="0" aria-label="Maximum price" placeholder="Max" value={maxPrice} onChange={(event) => setMaxPrice(event.target.value)} />
        </div>
        <Button type="button" size="sm" className="w-full" onClick={applyPrice}>Apply price</Button>
      </fieldset>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">Created date</legend>
        <Input type="date" aria-label="Created from" value={filters.created_from || ""} onChange={(event) => setFilter("created_from", event.target.value)} />
        <Input type="date" aria-label="Created to" value={filters.created_to || ""} onChange={(event) => setFilter("created_to", event.target.value)} />
      </fieldset>

      {facets.map((facet) => {
        const selected = new Set((filters[facet.key] || "").split(",").filter(Boolean));
        return (
          <fieldset key={facet.key} className="space-y-1">
            <legend className="mb-1 text-sm font-medium">{facet.name}</legend>
            {facet.values.map((value) => (
              <button key={value.value} type="button" aria-pressed={selected.has(value.value)} onClick={() => toggleValue(facet.key, value.value)} className={cn("flex w-full items-center justify-between rounded-md px-2 py-1.5 text-sm", selected.has(value.value) ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted")}>
                <span>{value.label}</span><Badge variant="secondary">{value.count}</Badge>
              </button>
            ))}
          </fieldset>
        );
      })}
    </section>
  );
}
