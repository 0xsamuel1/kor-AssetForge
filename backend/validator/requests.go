package validator

// TokenizeAssetRequest defines the expected request body for tokenizing assets.
type TokenizeAssetRequest struct {
	IssuerAccount string            `json:"issuer_account" binding:"required,stellar_address"`
	Name          string            `json:"name" binding:"required,min=3,max=100,no_html"`
	Symbol        string            `json:"symbol" binding:"required,asset_symbol"`
	Description   string            `json:"description" binding:"omitempty,max=500,no_html"`
	AssetType     string            `json:"asset_type" binding:"required,asset_type"`
	TotalSupply   int64             `json:"total_supply" binding:"required,gt=0"`
	Metadata      map[string]string `json:"metadata" binding:"omitempty"`
	Fractions     uint64            `json:"fractions" binding:"gte=0"`
}

// ListAssetSaleRequest defines the expected marketplace listing body.
type ListAssetSaleRequest struct {
	AssetID      uint   `json:"asset_id" binding:"required,gt=0"`
	SellerAddr   string `json:"seller_address" binding:"required,stellar_address"`
	Amount       int64  `json:"amount" binding:"required,gt=0"`
	PricePerUnit int64  `json:"price_per_unit" binding:"required,gt=0"`
}

// TransferAssetRequest defines the expected transfer body.
type TransferAssetRequest struct {
	AssetID     uint   `json:"asset_id" binding:"required,gt=0"`
	FromAddress string `json:"from_address" binding:"required,stellar_address"`
	ToAddress   string `json:"to_address" binding:"required,stellar_address"`
	Amount      int64  `json:"amount" binding:"required,gt=0"`
	Memo        string `json:"memo" binding:"omitempty,max=500,no_html"`
}

// PaginationQuery validates optional pagination parameters.
type PaginationQuery struct {
	Page  int `form:"page" binding:"omitempty,min=1"`
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
}

// TransactionQuery validates transaction list query parameters.
type TransactionQuery struct {
	AssetID uint   `form:"asset_id" binding:"omitempty,gt=0"`
	Memo    string `form:"memo" binding:"omitempty,max=500"`
	Page    int    `form:"page" binding:"omitempty,min=1"`
	Limit   int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// AssetIDUri validates asset ID path parameters.
type AssetIDUri struct {
	ID uint `uri:"id" binding:"required,gt=0"`
}

// Setup2FARequest validates 2FA setup request.
type Setup2FARequest struct {
	Token string `json:"token" binding:"required,len=6"`
}

// Verify2FARequest validates 2FA verification request during login.
type Verify2FARequest struct {
	UserID    uint   `json:"user_id" binding:"required"`
	TOTPToken string `json:"totp_token" binding:"required"`
}

// Disable2FARequest validates 2FA disable request.
type Disable2FARequest struct {
	Password  string `json:"password" binding:"required"`
	TOTPToken string `json:"totp_token" binding:"required"`
}

// UpdateMetadataRequest validates NFT metadata update request.
type UpdateMetadataRequest struct {
	AssetID      uint   `json:"asset_id" binding:"required"`
	MetadataURI  string `json:"metadata_uri" binding:"required,metadata_uri"`
	MetadataHash string `json:"metadata_hash" binding:"required,metadata_hash"`
}

// MakeImmutableRequest validates make-immutable request.
type MakeImmutableRequest struct {
	AssetID uint `json:"asset_id" binding:"required"`
}

// BatchOperationRequest validates a single batch operation.
type BatchOperationRequest struct {
	Type        string                 `json:"type" binding:"required,oneof=transfer mint burn list cancel_listing"`
	AssetID     uint                   `json:"asset_id" binding:"required"`
	FromAddress string                 `json:"from_address" binding:"omitempty,stellar_address"`
	ToAddress   string                 `json:"to_address" binding:"omitempty,stellar_address"`
	Amount      int64                  `json:"amount" binding:"omitempty,gt=0"`
	ExtraParams map[string]interface{} `json:"extra_params,omitempty"`
}

// BatchTransactionRequest validates a batch transaction request.
type BatchTransactionRequest struct {
	Operations []BatchOperationRequest `json:"operations" binding:"required,min=1,max=50"`
}

// BatchStatusQuery validates batch status query parameters.
type BatchStatusQuery struct {
	Status string `form:"status" binding:"omitempty,oneof=pending processing completed failed rolled_back"`
	Page   int    `form:"page" binding:"omitempty,min=1"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
}
