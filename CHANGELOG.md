# Changelog

All notable changes to kor-AssetForge will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial API endpoint definitions for asset management
- RESTful API for asset tokenization and marketplace operations
- Database schema for asset registry and ownership tracking
- Soroban smart contract integration for on-chain transactions
- Multi-signature support for compliance workflows

### Changed
- Updated API documentation with detailed endpoint specifications
- Enhanced error handling and validation across endpoints

### Security
- Added input validation for all API endpoints
- Implemented rate limiting on marketplace operations
- Enhanced JWT token authentication mechanism

## [0.1.0] - 2026-10-16

### Added
- Initial project setup with Go backend and React Native mobile app
- Soroban smart contracts for asset tokenization
- PostgreSQL database with GORM integration
- Basic asset creation and listing endpoints
- User authentication with email/password and biometric support
- Marketplace browsing and trading functionality
- Portfolio tracking and performance analytics
- Stellar wallet integration for asset transfers
- Compliance reporting and audit trail functionality
- Docker Compose for local development environment

### Features
- Fractional ownership of real-world assets (RWAs)
- On-chain marketplace for asset trading
- Asset whitelisting and transfer restrictions
- Multi-asset registry with per-asset analytics
- Accredited investor verification workflow
- KYC/AML security compliance
- Real-time price feeds and market data

### Infrastructure
- Docker containerization for all services
- PostgreSQL for off-chain metadata
- Stellar Testnet/Futurenet support
- Automated deployment scripts

## Future Plans

- [ ] GraphQL API implementation
- [ ] Advanced analytics dashboard
- [ ] Mobile app push notifications enhancement
- [ ] Multi-chain asset bridging
- [ ] Decentralized governance for asset management
- [ ] Automated market maker (AMM) for asset pairs

[Unreleased]: https://github.com/parkerwinner/kor-AssetForge/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/parkerwinner/kor-AssetForge/releases/tag/v0.1.0
