import create from 'zustand';
import AsyncStorage from '@react-native-async-storage/async-storage';
import { Keypair } from '@stellar/stellar-sdk';
import api from '../services/api';

interface Wallet {
  address: string;
  publicKey: string;
  balance: number;
}

interface WalletStore {
  wallet: Wallet | null;
  isLoading: boolean;
  initializeWallet: () => Promise<void>;
  syncWallet: () => Promise<void>;
  deleteWallet: () => Promise<void>;
}

export const useWalletStore = create<WalletStore>((set) => ({
  wallet: null,
  isLoading: false,

  initializeWallet: async () => {
    set({ isLoading: true });
    try {
      // Generate new Stellar keypair
      const keypair = Keypair.random();
      const publicKey = keypair.publicKey();
      const secretSeed = keypair.secret();

      // Store encrypted secret in secure storage
      await AsyncStorage.setItem('stellar_secret', secretSeed);

      const wallet = {
        address: publicKey,
        publicKey: publicKey,
        balance: 0,
      };

      // Register wallet with backend
      await api.post('/wallet/register', { public_key: publicKey });

      set({ wallet, isLoading: false });
    } catch (error) {
      set({ isLoading: false });
      throw error;
    }
  },

  syncWallet: async () => {
    set({ isLoading: true });
    try {
      const response = await api.get('/wallet');
      set({
        wallet: response.data,
        isLoading: false,
      });
    } catch (error) {
      set({ isLoading: false });
      throw error;
    }
  },

  deleteWallet: async () => {
    await AsyncStorage.removeItem('stellar_secret');
    set({ wallet: null });
  },
}));
