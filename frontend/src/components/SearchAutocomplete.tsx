'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { assetApi, AutocompleteResult } from "@/lib/asset-api";
import { cn } from "@/lib/utils";
import { Clock, Loader2, Search, Sparkles, TrendingUp, X } from "lucide-react";

const RECENT_SEARCHES_KEY = "assetforge.recent-searches";

interface SearchAutocompleteProps {
  onSearch: (query: string) => void;
  className?: string;
  initialValue?: string;
}

export function SearchAutocomplete({ onSearch, className, initialValue = "" }: SearchAutocompleteProps) {
  const [query, setQuery] = useState(initialValue);
  const [data, setData] = useState<AutocompleteResult>({ suggestions: [], recent: [], popular: [] });
  const [localRecent, setLocalRecent] = useState<string[]>(() => {
    if (typeof window === "undefined") return [];
    try {
      return JSON.parse(localStorage.getItem(RECENT_SEARCHES_KEY) || "[]").slice(0, 5);
    } catch {
      localStorage.removeItem(RECENT_SEARCHES_KEY);
      return [];
    }
  });
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);

  const fetchSuggestions = useCallback(async (value: string) => {
    setIsLoading(true);
    try {
      setData(await assetApi.getAutocomplete(value));
    } catch {
      setData({ suggestions: [], recent: [], popular: [] });
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void fetchSuggestions(query), 250);
    return () => window.clearTimeout(timer);
  }, [query, fetchSuggestions]);

  useEffect(() => {
    const close = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) setIsOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, []);

  const recent = useMemo(() => {
    const values = [...(data.recent || []), ...localRecent];
    return values.filter((value, index) =>
      values.findIndex((candidate) => candidate.toLowerCase() === value.toLowerCase()) === index &&
      (!query || value.toLowerCase().includes(query.toLowerCase()))
    ).slice(0, 5);
  }, [data.recent, localRecent, query]);
  const entries = useMemo(() => [
    ...data.suggestions.map((value) => ({ value, kind: "suggestion" as const })),
    ...recent.map((value) => ({ value, kind: "recent" as const })),
    ...(data.popular || []).map((value) => ({ value, kind: "popular" as const })),
  ].filter((entry, index, all) =>
    all.findIndex((candidate) => candidate.value.toLowerCase() === entry.value.toLowerCase()) === index
  ), [data.suggestions, data.popular, recent]);

  const submit = (value: string) => {
    const normalized = value.trim();
    setQuery(normalized);
    if (normalized) {
      const next = [normalized, ...localRecent.filter((item) => item.toLowerCase() !== normalized.toLowerCase())].slice(0, 5);
      setLocalRecent(next);
      localStorage.setItem(RECENT_SEARCHES_KEY, JSON.stringify(next));
    }
    onSearch(normalized);
    setIsOpen(false);
  };

  return (
    <div ref={containerRef} className={cn("relative", className)}>
      <form onSubmit={(event) => { event.preventDefault(); submit(query); }} role="search">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            value={query}
            onChange={(event) => { setQuery(event.target.value); setSelectedIndex(-1); setIsOpen(true); }}
            onFocus={() => { setIsOpen(true); void fetchSuggestions(query); }}
            onKeyDown={(event) => {
              if (event.key === "ArrowDown") { event.preventDefault(); setSelectedIndex((index) => Math.min(index + 1, entries.length - 1)); }
              if (event.key === "ArrowUp") { event.preventDefault(); setSelectedIndex((index) => Math.max(index - 1, -1)); }
              if (event.key === "Enter" && selectedIndex >= 0) { event.preventDefault(); submit(entries[selectedIndex].value); }
              if (event.key === "Escape") setIsOpen(false);
            }}
            placeholder="Search assets, tokens, categories..."
            aria-label="Search assets"
            role="combobox"
            aria-expanded={isOpen}
            aria-controls="search-autocomplete-options"
            aria-activedescendant={selectedIndex >= 0 ? `search-autocomplete-${selectedIndex}` : undefined}
            className="pl-9 pr-24"
          />
          <div className="absolute right-2 top-1/2 flex -translate-y-1/2 items-center gap-1">
            {isLoading && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" aria-hidden="true" />}
            {query && <Button type="button" variant="ghost" size="icon" className="h-6 w-6" aria-label="Clear search" onClick={() => submit("")}><X className="h-3 w-3" /></Button>}
            <Button type="submit" size="sm" className="h-7">Search</Button>
          </div>
        </div>
      </form>

      {isOpen && (entries.length > 0 || data.did_you_mean) && (
        <div id="search-autocomplete-options" role="listbox" className="absolute top-full z-50 mt-1 max-h-80 w-full overflow-auto rounded-lg border bg-popover p-1 shadow-lg">
          {data.did_you_mean && (
            <button type="button" className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-muted" onClick={() => submit(data.did_you_mean!)}>
              <Sparkles className="h-4 w-4 text-primary" /> Did you mean <strong>{data.did_you_mean}</strong>?
            </button>
          )}
          {entries.map((entry, index) => (
            <button id={`search-autocomplete-${index}`} role="option" aria-selected={selectedIndex === index} type="button" key={`${entry.kind}-${entry.value}`} onClick={() => submit(entry.value)} className={cn("flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-muted", selectedIndex === index && "bg-muted")}>
              {entry.kind === "recent" ? <Clock className="h-4 w-4 text-muted-foreground" /> : entry.kind === "popular" ? <TrendingUp className="h-4 w-4 text-muted-foreground" /> : <Search className="h-4 w-4 text-muted-foreground" />}
              <span>{entry.value}</span>
              {entry.kind !== "suggestion" && <span className="ml-auto text-xs capitalize text-muted-foreground">{entry.kind}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
