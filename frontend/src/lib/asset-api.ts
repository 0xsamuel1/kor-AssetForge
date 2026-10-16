export interface PricePoint {
  timestamp: number;
  price: number;
  volume: number;
}

export interface OwnershipEntry {
  address: string;
  percentage: number;
  label: string;
}

export interface TransactionEntry {
  id: string;
  type: "buy" | "sell" | "transfer" | "mint" | "burn" | "stake" | "unstake" | "dividend";
  from: string;
  to: string;
  amount: number;
  price?: number;
  fee?: number;
  assetId: string;
  assetName?: string;
  assetSymbol?: string;
  timestamp: number;
  txHash: string;
  status: "completed" | "pending" | "failed";
}

export interface UserTransactionParams {
  search?: string;
  type?: string;
  status?: string;
  startDate?: string;
  endDate?: string;
  minAmount?: string;
  maxAmount?: string;
  page?: number;
  limit?: number;
  cursor?: string;
}

export interface TransactionPage {
  transactions: TransactionEntry[];
  total: number;
  hasMore: boolean;
  nextCursor?: string;
}

export interface AssetDetail {
  id: string;
  code: string;
  issuer: string;
  name: string;
  description: string;
  totalSupply: string;
  decimals: number;
  price: number;
  priceChange24h: number;
  marketCap: number;
  volume24h: number;
  allTimeHigh: number;
  allTimeLow: number;
  createdAt: number;
  verified: boolean;
  documents: {
    title: string;
    url: string;
    type: string;
  }[];
  metadata: {
    category: string;
    location: string;
    condition: string;
    tags: string[];
  };
}

export interface PortfolioData {
  totalValue: number;
  pnl24h: number;
  pnlPercentage: number;
  holdings: {
    assetId: string;
    name: string;
    symbol: string;
    balance: number;
    value: number;
    change24h: number;
    allocation: number;
  }[];
  recentActivity: {
    id: string;
    type: string;
    assetName: string;
    amount: number;
    timestamp: number;
  }[];
  pendingActions: {
    id: string;
    action: string;
    assetName: string;
    details: string;
    created: number;
  }[];
  performanceHistory: PricePoint[];
}

export interface SearchResult {
  id: string;
  name: string;
  symbol: string;
  type: string;
  price: number;
  change24h: number;
  volume: number;
  location: string;
  status: string;
  verified: boolean;
}

export interface FilterFacet {
  name: string;
  key: string;
  values: {
    label: string;
    value: string;
    count: number;
  }[];
}

export interface AutocompleteResult {
  suggestions: string[];
  did_you_mean?: string;
  recent?: string[];
  popular?: string[];
}

class AssetApiService {
  private backendUrl: string;

  constructor() {
    this.backendUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  }

  async getAssetDetail(id: string): Promise<AssetDetail> {
    const res = await fetch(`${this.backendUrl}/api/v1/assets/${id}`);
    if (!res.ok) throw new Error("Failed to fetch asset detail");
    return res.json();
  }

  async getPriceHistory(assetId: string, period: "7d" | "30d" | "1y"): Promise<PricePoint[]> {
    const res = await fetch(`${this.backendUrl}/api/v1/assets/${assetId}/price-history?period=${period}`);
    if (!res.ok) throw new Error("Failed to fetch price history");
    return res.json();
  }

  async getOwnershipDistribution(assetId: string): Promise<OwnershipEntry[]> {
    const res = await fetch(`${this.backendUrl}/api/v1/assets/${assetId}/ownership`);
    if (!res.ok) throw new Error("Failed to fetch ownership data");
    return res.json();
  }

  async getTransactionHistory(assetId: string, limit: number = 20): Promise<TransactionEntry[]> {
    const res = await fetch(`${this.backendUrl}/api/v1/assets/${assetId}/transactions?limit=${limit}`);
    if (!res.ok) throw new Error("Failed to fetch transactions");
    return res.json();
  }

  async getRelatedAssets(assetId: string): Promise<SearchResult[]> {
    const res = await fetch(`${this.backendUrl}/api/v1/assets/${assetId}/related`);
    if (!res.ok) throw new Error("Failed to fetch related assets");
    return res.json();
  }

  async searchAssets(query: string, filters: Record<string, string>): Promise<SearchResult[]> {
    const params = new URLSearchParams({ q: query });
    Object.entries(filters).forEach(([key, value]) => {
      if (["asset_type", "category", "tag", "location"].includes(key)) {
        value.split(",").filter(Boolean).forEach((item) => params.append(key, item));
      } else {
        params.set(key, value);
      }
    });
    const res = await fetch(`${this.backendUrl}/api/v1/search/assets?${params}`);
    if (!res.ok) throw new Error("Failed to search assets");
    const payload = await res.json();
    const assets = Array.isArray(payload) ? payload : payload.assets || [];
    return assets.map((asset: Record<string, unknown>) => {
      let metadata: Record<string, unknown> = {};
      try {
        metadata = typeof asset.metadata === "string" ? JSON.parse(asset.metadata) : (asset.metadata as Record<string, unknown>) || {};
      } catch {}
      return {
        id: String(asset.id), name: String(asset.name || ""), symbol: String(asset.symbol || ""),
        type: String(asset.asset_type || asset.type || ""), price: Number(asset.current_price || asset.price || 0),
        change24h: Number(asset.change_24h || 0), volume: Number(asset.volume || 0),
        location: String(metadata.location || asset.location || ""), status: String(asset.status || "active"),
        verified: Boolean(asset.verified),
      } satisfies SearchResult;
    });
  }

  async getFilterFacets(): Promise<FilterFacet[]> {
    const res = await fetch(`${this.backendUrl}/api/v1/search/facets`);
    if (!res.ok) throw new Error("Failed to fetch filter facets");
    return res.json();
  }

  async getAutocompleteSuggestions(query: string): Promise<string[]> {
    const res = await fetch(`${this.backendUrl}/api/v1/search/suggestions?q=${encodeURIComponent(query)}`);
    if (!res.ok) throw new Error("Failed to fetch suggestions");
    const data = await res.json();
    return data.suggestions;
  }

  async getAutocomplete(query: string): Promise<AutocompleteResult> {
    const res = await fetch(`${this.backendUrl}/api/v1/search/autocomplete?q=${encodeURIComponent(query)}`);
    if (!res.ok) throw new Error("Failed to fetch autocomplete results");
    return res.json();
  }

  async getDashboardData(): Promise<PortfolioData> {
    const res = await fetch(`${this.backendUrl}/api/v1/dashboard`);
    if (!res.ok) throw new Error("Failed to fetch dashboard data");
    return res.json();
  }

  async exportTransactionsCsv(assetId?: string, startDate?: string, endDate?: string): Promise<Blob> {
    const params = new URLSearchParams();
    if (assetId) params.set("asset_id", assetId);
    if (startDate) params.set("start", startDate);
    if (endDate) params.set("end", endDate);
    const res = await fetch(`${this.backendUrl}/api/v1/dashboard/export?${params}`);
    if (!res.ok) throw new Error("Failed to export transactions");
    return res.blob();
  }

  async getUserTransactions(params: UserTransactionParams = {}): Promise<TransactionPage> {
    const query = new URLSearchParams();
    if (params.search) query.set("q", params.search);
    if (params.type) query.set("type", params.type);
    if (params.status) query.set("status", params.status);
    if (params.startDate) query.set("start", params.startDate);
    if (params.endDate) query.set("end", params.endDate);
    if (params.minAmount) query.set("min_amount", params.minAmount);
    if (params.maxAmount) query.set("max_amount", params.maxAmount);
    if (params.page !== undefined) query.set("page", String(params.page));
    if (params.limit) query.set("limit", String(params.limit));
    if (params.cursor) query.set("cursor", params.cursor);
    const res = await fetch(`${this.backendUrl}/api/v1/user/transactions?${query}`);
    if (!res.ok) throw new Error("Failed to fetch user transactions");
    return res.json();
  }

  async getTransactionReceipt(txId: string): Promise<Blob> {
    const res = await fetch(`${this.backendUrl}/api/v1/user/transactions/${txId}/receipt`);
    if (!res.ok) throw new Error("Failed to fetch transaction receipt");
    return res.blob();
  }

  async saveSearch(name: string, query: string, filters: Record<string, string>): Promise<void> {
    const res = await fetch(`${this.backendUrl}/api/v1/search/saved`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, query, filters }),
    });
    if (!res.ok) throw new Error("Failed to save search");
  }
}

export const assetApi = new AssetApiService();
