# KOR AssetForge Mobile App - Complete Guide

This document provides comprehensive documentation for the KOR AssetForge mobile application, including setup, features, architecture, and integration details.

## Table of Contents

1. [Overview](#overview)
2. [Getting Started](#getting-started)
3. [Features](#features)
4. [User Guide](#user-guide)
5. [Architecture](#architecture)
6. [Development](#development)
7. [Security](#security)
8. [API Integration](#api-integration)
9. [Troubleshooting](#troubleshooting)

## Overview

KOR AssetForge is a cross-platform mobile application built with React Native, enabling users to:
- Authenticate and manage accounts
- Browse and trade tokenized real-world assets
- Manage Stellar wallets
- Track investment portfolios
- Receive real-time market notifications

### Supported Platforms

- **iOS**: 12.0 and above
- **Android**: API level 21 (Android 5.0) and above

### Technology Stack

- **Framework**: React Native 0.73+
- **State Management**: Zustand
- **Navigation**: React Navigation v6
- **HTTP Client**: Axios
- **Blockchain Integration**: Stellar SDK
- **Authentication**: Biometric & JWT tokens
- **Push Notifications**: Firebase Cloud Messaging (FCM) / Apple Push Notification service (APNs)

## Getting Started

### Prerequisites

#### Development Environment

- **Node.js**: 16.x or later
- **npm** or **yarn**: Latest version
- **Watchman**: For macOS (file system watcher)

#### Platform-Specific Requirements

**iOS (macOS only):**
- Xcode 13.0+
- CocoaPods
- iOS deployment target: 12.0+

**Android:**
- Android Studio 2021.3+
- Java Development Kit (JDK) 11+
- Android SDK level 31+

### Installation Steps

1. **Clone the repository:**
```bash
git clone https://github.com/parkerwinner/kor-AssetForge.git
cd kor-AssetForge/mobile
```

2. **Install dependencies:**
```bash
npm install
# or
yarn install
```

3. **Set up environment variables:**
```bash
cp .env.example .env
```

Configure your `.env` file:
```
REACT_APP_API_URL=http://localhost:8080/api
REACT_APP_STELLAR_NETWORK=testnet
REACT_APP_STELLAR_HORIZON_URL=https://horizon-testnet.stellar.org
REACT_APP_FCM_SENDER_ID=your_fcm_sender_id
REACT_APP_APP_VERSION=0.1.0
```

4. **iOS Setup (macOS):**
```bash
cd ios
pod install
cd ..
npm run ios
```

5. **Android Setup:**
```bash
npm run android
```

6. **Run Development Server:**
```bash
npm start
```

## Features

### Authentication

#### Email & Password Registration
- Secure email verification
- Password strength requirements (min 8 chars, mixed case, numbers, special chars)
- Automatic session timeout after 15 minutes of inactivity

#### Biometric Authentication
- **Fingerprint Recognition**: Android Touch ID / iOS Face ID
- **Face Recognition**: Device camera-based face unlock
- Secure credential storage in device keychain
- Fallback to password if biometrics fail

**Usage Example:**
```typescript
const { isBiometricAvailable, authenticate } = useBiometrics();

const handleBiometricAuth = async () => {
  try {
    const result = await authenticate();
    if (result.success) {
      // User authenticated
      navigate('Home');
    }
  } catch (error) {
    console.error('Biometric auth failed', error);
  }
};
```

### Asset Marketplace

#### Browse Assets
- View all available tokenized assets
- Filter by asset type (real estate, art, commodities, etc.)
- Search by name or issuer
- Sort by price, performance, or newest

#### Asset Details
- Detailed asset information and metrics
- Historical price charts
- Ownership breakdown
- Issuer information and certifications
- Trading volume and liquidity metrics

#### Trading Interface
- Real-time price feeds
- Easy buy/sell interface
- Quantity and price entry
- Trade confirmation with summary
- Order history and receipts

### Portfolio Management

#### Holdings Overview
- Current asset holdings with quantity
- Real-time portfolio value
- Total return and percentage gain/loss
- Asset allocation pie chart
- Performance by asset

#### Portfolio Analytics
- Historical performance graphs
- Return statistics (7d, 30d, 90d, 1y, all-time)
- Volatility analysis
- Asset correlation matrix
- Risk assessment

#### Export Options
- Export holdings as CSV
- Generate tax reports
- Export transaction history
- Share portfolio summary

### Wallet Management

#### Stellar Wallet Integration
- Create or import Stellar accounts
- View account balances
- Send and receive assets
- Transaction history
- QR code for sharing wallet address

#### Security Features
- Keypair encryption
- Secure seed phrase storage
- Recovery phrase backup
- Two-factor authentication options

### Notifications

#### Push Notifications
- Trade execution updates
- Asset price alerts (customizable)
- Portfolio milestone notifications
- Platform announcements
- Market analysis digests

#### Notification Preferences
- Enable/disable by type
- Quiet hours scheduling
- Customize alert thresholds
- Notification history

## User Guide

### First-Time Setup

1. **Create Account**
   - Enter email address
   - Set strong password
   - Verify email via confirmation link

2. **Enable Biometric Authentication** (Recommended)
   - Navigate to Settings > Security
   - Enable Face ID / Touch ID
   - Confirm with your biometric

3. **Complete KYC Verification**
   - Navigate to Profile > Verification
   - Upload required documents
   - Wait for verification (typically 24-48 hours)

4. **Set Up Stellar Wallet**
   - Create new wallet or import existing
   - Securely store recovery phrase
   - Fund wallet with initial amount

### Trading Assets

1. **Browse Marketplace**
   - Tap "Marketplace" tab
   - Scroll through available assets or use search

2. **View Asset Details**
   - Tap an asset to see full details
   - Review price history and metrics
   - Check ownership breakdown

3. **Execute Trade**
   - Tap "Buy" or "Sell"
   - Enter quantity desired
   - Review trade summary
   - Confirm transaction

4. **Monitor Trade Status**
   - Trade appears in "Orders" section
   - Receive notification when confirmed
   - View transaction hash on Stellar network

### Managing Alerts

1. **Set Price Alerts**
   - Hold on an asset in portfolio
   - Select "Create Alert"
   - Set target price and conditions
   - Choose notification method

2. **Custom Notifications**
   - Settings > Notifications
   - Toggle notification types
   - Set quiet hours (e.g., 10 PM - 8 AM)
   - Configure alert thresholds

## Architecture

### Project Structure

```
mobile/
├── src/
│   ├── App.tsx                 # Root component & navigation setup
│   ├── screens/
│   │   ├── auth/              # Login, register, password recovery
│   │   ├── marketplace/       # Asset browsing & details
│   │   ├── trading/           # Buy/sell interface
│   │   ├── portfolio/         # Holdings & analytics
│   │   ├── wallet/            # Stellar wallet management
│   │   └── profile/           # User settings & preferences
│   ├── components/            # Reusable components
│   │   ├── AssetCard.tsx
│   │   ├── PriceChart.tsx
│   │   ├── PortfolioWidget.tsx
│   │   └── ...
│   ├── store/                 # Zustand state management
│   │   ├── authStore.ts       # Auth state (user, tokens)
│   │   ├── portfolioStore.ts  # Portfolio & holdings
│   │   ├── walletStore.ts     # Wallet state
│   │   └── marketStore.ts     # Market data & assets
│   ├── services/              # API & external services
│   │   ├── api.ts             # Axios instance & endpoints
│   │   ├── stellarService.ts  # Stellar SDK integration
│   │   └── notificationService.ts
│   ├── hooks/                 # Custom React hooks
│   │   ├── useBiometrics.ts
│   │   ├── usePortfolio.ts
│   │   └── useMarketData.ts
│   ├── utils/                 # Utility functions
│   │   ├── formatters.ts      # Number/date formatting
│   │   ├── validators.ts      # Input validation
│   │   └── helpers.ts
│   ├── types/                 # TypeScript types
│   │   ├── api.ts
│   │   ├── models.ts
│   │   └── navigation.ts
│   └── styles/                # Theme & global styles
├── package.json
├── tsconfig.json
└── README.md
```

### State Management with Zustand

Example store structure:

```typescript
// store/portfolioStore.ts
import create from 'zustand';

interface PortfolioStore {
  holdings: Asset[];
  totalValue: number;
  totalReturn: number;
  fetchPortfolio: (userId: string) => Promise<void>;
  addHolding: (asset: Asset) => void;
}

export const usePortfolioStore = create<PortfolioStore>((set) => ({
  holdings: [],
  totalValue: 0,
  totalReturn: 0,
  
  fetchPortfolio: async (userId) => {
    const data = await api.portfolio.get(userId);
    set({
      holdings: data.holdings,
      totalValue: data.totalValue,
      totalReturn: data.totalReturn,
    });
  },
  
  addHolding: (asset) => set((state) => ({
    holdings: [...state.holdings, asset],
  })),
}));
```

### Component Architecture

Components follow this pattern:

```typescript
// screens/marketplace/AssetList.tsx
import React, { useEffect } from 'react';
import { View, FlatList } from 'react-native';
import { useMarketStore } from '../../store/marketStore';
import AssetCard from '../../components/AssetCard';

export const AssetList: React.FC = () => {
  const { assets, loading, fetchAssets } = useMarketStore();

  useEffect(() => {
    fetchAssets();
  }, []);

  return (
    <View>
      <FlatList
        data={assets}
        renderItem={({ item }) => <AssetCard asset={item} />}
        keyExtractor={(item) => item.id}
        refreshing={loading}
        onRefresh={fetchAssets}
      />
    </View>
  );
};
```

## Development

### Running in Development Mode

```bash
# Terminal 1: Start Metro bundler
npm start

# Terminal 2: iOS
npm run ios

# Terminal 2: Android
npm run android
```

### Hot Reload

The app supports hot reload:
- Save a file to automatically reload the app
- For larger changes, press `r` in Metro terminal

### Debugging

#### React Native Debugger

```bash
# Install globally
npm install -g react-native-debugger

# Open debugger
react-native-debugger

# In your app, press Ctrl+M (Android) or Cmd+D (iOS)
# Select "Debug"
```

#### Console Logs

```typescript
console.log('Debug message');  // Shows in Metro terminal
console.warn('Warning');       // Yellow warning
console.error('Error');        // Red error
```

### Testing

#### Unit Tests

```bash
# Run tests
npm test

# Watch mode
npm test -- --watch

# Coverage
npm test -- --coverage
```

#### Component Testing with React Native Testing Library

```typescript
import { render, screen } from '@testing-library/react-native';
import AssetCard from './AssetCard';

test('renders asset name', () => {
  const asset = { id: '1', name: 'Test Asset', price: 100 };
  render(<AssetCard asset={asset} />);
  expect(screen.getByText('Test Asset')).toBeTruthy();
});
```

## Security

### Authentication Security

- **Passwords**: Bcrypt hashing, enforced complexity requirements
- **Tokens**: JWT with 1-hour expiry + refresh tokens (7 days)
- **Biometric**: Secure enclave storage, device-specific

### Data Security

- **HTTPS Only**: All API communication encrypted
- **Certificate Pinning**: Prevent man-in-the-middle attacks
- **Encryption at Rest**: Sensitive data encrypted on device

### Best Practices

1. **Never hardcode secrets** in code
2. **Use `.env` files** for configuration
3. **Validate user input** before submission
4. **Implement rate limiting** for sensitive operations
5. **Regular security audits** and dependency updates

## API Integration

### Axios Configuration

```typescript
// services/api.ts
import axios from 'axios';
import { useAuthStore } from '../store/authStore';

const api = axios.create({
  baseURL: process.env.REACT_APP_API_URL,
  timeout: 10000,
});

// Interceptor: Add auth token
api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Interceptor: Handle 401 errors
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout();
    }
    return Promise.reject(error);
  }
);

export default api;
```

### Making API Calls

```typescript
// Fetch assets
const fetchAssets = async () => {
  try {
    const response = await api.get('/assets', {
      params: { limit: 20, type: 'real_estate' }
    });
    return response.data.assets;
  } catch (error) {
    console.error('Failed to fetch assets', error);
    throw error;
  }
};
```

## Troubleshooting

### Common Issues

#### App Won't Start

**iOS:**
```bash
cd ios
rm -rf Pods Podfile.lock
pod install
cd ..
npm start
```

**Android:**
```bash
npm start -- --reset-cache
npm run android
```

#### Biometric Not Working

- Ensure device has biometric sensors
- Check Settings > Security > Biometric enabled
- Clear app cache: Settings > Apps > KOR AssetForge > Clear Cache

#### Network Errors

- Verify API endpoint in `.env`
- Check internet connection
- Verify API server is running
- Check firewall/proxy settings

#### Blank Screen

- Clear Metro cache: `npm start -- --reset-cache`
- Restart Metro bundler
- Rebuild app: `npm run android` / `npm run ios`

### Logs & Debugging

Access native logs:

**iOS:**
```bash
xcrun simctl spawn booted log stream --predicate 'process == "KORAssetForge"'
```

**Android:**
```bash
adb logcat | grep -i "assetforge"
```

## Support & Resources

- **GitHub**: https://github.com/parkerwinner/kor-AssetForge
- **Slack**: [Join our Slack](https://slack.assetforge.io)
- **Email**: support@assetforge.io
- **Docs**: https://docs.assetforge.io
- **Issues**: https://github.com/parkerwinner/kor-AssetForge/issues

