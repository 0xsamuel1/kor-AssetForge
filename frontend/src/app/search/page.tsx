'use client'

import { Suspense, useState, useEffect, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Header } from "@/components/Header";
import { SearchBar } from "@/components/SearchBar";
import { SearchFilters } from "@/components/SearchFilters";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { assetApi, SearchResult } from "@/lib/asset-api";
import { formatCurrency } from "@/lib/utils";
import { StellarWallet, stellarService } from "@/lib/stellar";
import {
  Building2, TrendingUp, TrendingDown, Shield,
  MapPin, Bookmark, BookmarkCheck,
} from "lucide-react";
import Link from "next/link";
import { CardGridSkeleton, PageSkeleton } from "@/components/LoadingSkeleton";

function SearchContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const [results, setResults] = useState<SearchResult[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [query, setQuery] = useState(searchParams.get("q") || "");
  const [filters, setFilters] = useState<Record<string, string>>(() => {
    const f: Record<string, string> = {};
    searchParams.forEach((value, key) => {
      if (key !== "q") f[key] = value;
    });
    return f;
  });
  const [savedSearches, setSavedSearches] = useState(false);

  useEffect(() => {
    const connect = async () => {
      try {
        const w = await stellarService.connectWallet();
        setWallet(w);
      } catch {}
    };
    connect();
  }, []);

  const updateUrl = useCallback((q: string, f: Record<string, string>) => {
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    Object.entries(f).forEach(([k, v]) => params.set(k, v));
    router.replace(`/search?${params.toString()}`, { scroll: false });
  }, [router]);

  const performSearch = useCallback(async (q: string, f: Record<string, string>) => {
    setIsLoading(true);
    try {
      const data = await assetApi.searchAssets(q, f);
      setResults(data);
    } catch {
      console.error("Search failed");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
	const timer = window.setTimeout(() => {
		void performSearch(query, filters);
		updateUrl(query, filters);
	}, 0);
	return () => window.clearTimeout(timer);
  }, [query, filters, performSearch, updateUrl]);

  const handleSearch = (q: string) => {
    setQuery(q);
  };

  const handleFiltersChange = (f: Record<string, string>) => {
    setFilters(f);
  };

  const handleSaveSearch = async () => {
    try {
      await assetApi.saveSearch(`Search: ${query || "All"}`, query, filters);
      setSavedSearches(true);
      setTimeout(() => setSavedSearches(false), 2000);
    } catch {
      console.error("Failed to save search");
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type.toLowerCase()) {
      case "real_estate": return "bg-blue-500/10 text-blue-500";
      case "commodity": return "bg-yellow-500/10 text-yellow-500";
      case "equity": return "bg-green-500/10 text-green-500";
      case "bond": return "bg-purple-500/10 text-purple-500";
      default: return "bg-muted text-muted-foreground";
    }
  };

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />

      <main className="container mx-auto px-4 py-8">
        <div className="mb-6">
          <SearchBar onSearch={handleSearch} initialValue={query} />
        </div>

        {query && (
          <div className="flex items-center gap-2 mb-4">
            <span className="text-sm text-muted-foreground">
              {results.length} results for &quot;{query}&quot;
            </span>
            <Button variant="ghost" size="sm" onClick={handleSaveSearch} className="h-7 text-xs">
              {savedSearches ? (
                <BookmarkCheck className="h-3.5 w-3.5 mr-1 text-green-500" />
              ) : (
                <Bookmark className="h-3.5 w-3.5 mr-1" />
              )}
              {savedSearches ? "Saved!" : "Save Search"}
            </Button>
          </div>
        )}

        <div className="flex gap-6">
          {/* Sidebar Filters */}
          <aside className="w-64 shrink-0 hidden lg:block">
            <div className="sticky top-4">
              <SearchFilters filters={filters} onFiltersChange={handleFiltersChange} />
            </div>
          </aside>

          {/* Results */}
          <div className="flex-1 min-w-0">
            {isLoading ? (
              <CardGridSkeleton count={6} className="lg:grid-cols-2 xl:grid-cols-3" label="Searching assets" />
            ) : results.length === 0 ? (
              <div className="text-center py-12">
                <Building2 className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
                <h3 className="text-lg font-semibold mb-2">No Results Found</h3>
                <p className="text-muted-foreground">
                  {query ? "Try adjusting your search terms or filters." : "Enter a search term to find assets."}
                </p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {results.map((result) => (
                  <Link key={result.id} href={`/assets/${result.id}`}>
                    <Card className="hover:shadow-md transition-shadow cursor-pointer h-full">
                      <CardHeader className="pb-3">
                        <div className="flex items-start justify-between">
                          <div className="flex items-center gap-3">
                            <div className={`h-10 w-10 rounded-lg flex items-center justify-center ${getTypeIcon(result.type)}`}>
                              <Building2 className="h-5 w-5" />
                            </div>
                            <div>
                              <CardTitle className="text-base">{result.name}</CardTitle>
                              <CardDescription className="text-sm">{result.symbol}</CardDescription>
                            </div>
                          </div>
                          {result.verified && (
                            <Badge variant="secondary" className="gap-1 shrink-0">
                              <Shield className="h-3 w-3" /> Verified
                            </Badge>
                          )}
                        </div>
                      </CardHeader>
                      <CardContent>
                        <div className="space-y-2">
                          <div className="flex justify-between items-center">
                            <span className="text-lg font-bold">{formatCurrency(result.price)}</span>
                            <span className={`flex items-center text-sm ${result.change24h >= 0 ? "text-green-500" : "text-red-500"}`}>
                              {result.change24h >= 0 ? <TrendingUp className="h-3.5 w-3.5 mr-1" /> : <TrendingDown className="h-3.5 w-3.5 mr-1" />}
                              {result.change24h >= 0 ? "+" : ""}{result.change24h}%
                            </span>
                          </div>
                          <div className="flex items-center justify-between text-sm text-muted-foreground">
                            <div className="flex items-center gap-1">
                              <MapPin className="h-3.5 w-3.5" />
                              {result.location || "N/A"}
                            </div>
                            <Badge variant="outline" className="text-xs">
                              {result.status}
                            </Badge>
                          </div>
                          <div className="flex justify-between text-sm">
                            <span className="text-muted-foreground">24h Volume</span>
                            <span className="font-medium tabular-nums">{formatCurrency(result.volume)}</span>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  </Link>
                ))}
              </div>
            )}
          </div>
        </div>
      </main>
    </div>
  );
}

export default function SearchPage() {
  return (
    <Suspense fallback={<PageSkeleton label="Loading search" />}>
      <SearchContent />
    </Suspense>
  );
}
