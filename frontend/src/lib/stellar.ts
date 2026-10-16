import { Horizon } from "@stellar/stellar-sdk";
import * as freighterApi from "@stellar/freighter-api";
import { toWalletError, WalletError } from "@/lib/wallet/errors";
import { signWithFreighter } from "@/lib/wallet/freighter";

export interface StellarWallet {
  publicKey: string;
  connected: boolean;
}

export interface AssetInfo {
  id: string;
  code: string;
  issuer: string;
  name: string;
  description?: string;
  total_supply: string;
  decimals: number;
}

export interface Listing {
  id: string;
  asset_id: string;
  seller: string;
  price: string;
  amount: string;
  created_at: number;
  status: "active" | "sold" | "cancelled";
}

export interface KYCData {
  firstName: string;
  lastName: string;
  email: string;
  phone: string;
  country: string;
  address: string;
  dateOfBirth: string;
  idType: string;
  idNumber: string;
}

class StellarService {
  private server: Horizon.Server;
  private backendUrl: string;

  constructor() {
    this.server = new Horizon.Server("https://horizon-testnet.stellar.org");
    this.backendUrl =
      process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  }

  async connectWallet(): Promise<StellarWallet> {
    try {
      const response = await freighterApi.getAddress();
      if (response.error) {
        throw toWalletError("freighter", response.error, "connect");
      }
      if (!response.address) {
        // Freighter returns an empty address while locked or not yet allowed.
        throw new WalletError(
          "locked",
          "freighter",
          "Freighter is locked or hasn't shared an account with this site.",
        );
      }
      return {
        publicKey: response.address,
        connected: true,
      };
    } catch (error) {
      console.error("Failed to connect wallet:", error);
      throw toWalletError("freighter", error, "connect");
    }
  }

  async disconnectWallet(): Promise<void> {
    // Freighter doesn't have a disconnect method, we just clear local state
  }

  async getAccount(publicKey: string) {
    try {
      const account = await this.server.loadAccount(publicKey);
      return account;
    } catch (error) {
      console.error("Failed to load account:", error);
      throw new Error("Failed to load account from Stellar");
    }
  }

  async signTransaction(xdr: string): Promise<string> {
    try {
      const { address, error } = await freighterApi.getAddress();
      if (error) throw toWalletError("freighter", error, "sign");
      if (!address) {
        throw new WalletError(
          "locked",
          "freighter",
          "Unlock Freighter to sign this transaction.",
        );
      }
      return await signWithFreighter(xdr, address);
    } catch (error) {
      console.error("Failed to sign transaction:", error);
      throw toWalletError("freighter", error, "sign");
    }
  }

  // Backend API calls
  async getAssets(): Promise<AssetInfo[]> {
    try {
      const response = await fetch(`${this.backendUrl}/api/v1/assets`);
      if (!response.ok) throw new Error("Failed to fetch assets");
      return response.json();
    } catch (error) {
      console.error("Failed to get assets:", error);
      throw error;
    }
  }

  async getAsset(id: string): Promise<AssetInfo> {
    try {
      const response = await fetch(`${this.backendUrl}/api/v1/assets/${id}`);
      if (!response.ok) throw new Error("Failed to fetch asset");
      return response.json();
    } catch (error) {
      console.error("Failed to get asset:", error);
      throw error;
    }
  }

  async getListings(assetId?: string): Promise<Listing[]> {
    try {
      const url = assetId
        ? `${this.backendUrl}/api/v1/marketplace/listings?asset_id=${assetId}`
        : `${this.backendUrl}/api/v1/marketplace/listings`;
      const response = await fetch(url);
      if (!response.ok) throw new Error("Failed to fetch listings");
      return response.json();
    } catch (error) {
      console.error("Failed to get listings:", error);
      throw error;
    }
  }

  async createListing(
    assetId: string,
    price: string,
    amount: string,
    publicKey: string,
  ): Promise<Listing> {
    try {
      const response = await fetch(
        `${this.backendUrl}/api/v1/marketplace/list`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            asset_id: assetId,
            price,
            amount,
            seller: publicKey,
          }),
        },
      );
      if (!response.ok) throw new Error("Failed to create listing");
      return response.json();
    } catch (error) {
      console.error("Failed to create listing:", error);
      throw error;
    }
  }

  async purchaseAsset(listingId: string, publicKey: string): Promise<void> {
    try {
      const response = await fetch(
        `${this.backendUrl}/api/v1/marketplace/purchase`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            listing_id: listingId,
            buyer: publicKey,
          }),
        },
      );
      if (!response.ok) throw new Error("Failed to purchase asset");
    } catch (error) {
      console.error("Failed to purchase asset:", error);
      throw error;
    }
  }

  // KYC related methods
  async submitKYC(userData: KYCData, publicKey: string): Promise<void> {
    try {
      const response = await fetch(`${this.backendUrl}/api/v1/kyc/submit`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          ...userData,
          wallet_address: publicKey,
        }),
      });
      if (!response.ok) throw new Error("Failed to submit KYC");
    } catch (error) {
      console.error("Failed to submit KYC:", error);
      throw error;
    }
  }

  async getKYCStatus(publicKey: string): Promise<string> {
    try {
      const response = await fetch(
        `${this.backendUrl}/api/v1/kyc/status/${publicKey}`,
      );
      if (!response.ok) throw new Error("Failed to get KYC status");
      const data = await response.json();
      return data.status;
    } catch (error) {
      console.error("Failed to get KYC status:", error);
      throw error;
    }
  }
}

export const stellarService = new StellarService();
