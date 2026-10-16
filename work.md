#163 Create admin interface for email template customization
Repo Avatar
parkerwinner/kor-AssetForge
Description: Build system for managing email templates with variable substitution, preview functionality, and version control. Support multi-language templates.

File Changes:

backend/models/email_template.go - Create template models
backend/handlers/email_templates.go - Add template management endpoints
backend/services/email_service.go - Update to use dynamic templates
backend/migrations/sql/0022_create_email_templates.up.sql - Create template tables
Branch: feature/email-template-management

PR Title: Implement customizable email templates with multi-language support

Additional Info: Add template validation, implement WYSIWYG editor support, add A/B testing for email templates.

#167 Enable side-by-side comparison of multiple assets
Repo Avatar
parkerwinner/kor-AssetForge
Description: Create endpoint for comparing key metrics, attributes, and performance of multiple assets. Support comparison of 2-10 assets simultaneously.

File Changes:

backend/handlers/comparison.go - Create comparison endpoints
backend/services/comparison_service.go - Implement comparison logic
Branch: feature/asset-comparison

PR Title: Add asset comparison tool for side-by-side analysis

Additional Info: Add comparison result caching, implement comparison history, support custom comparison criteria.

#169 Calculate and track asset performance indicators
Repo Avatar
parkerwinner/kor-AssetForge
Description: Implement system to calculate ROI, appreciation rate, dividend yield, and other performance metrics for assets. Provide historical performance tracking.

File Changes:

backend/models/performance_metrics.go - Create metrics models
backend/services/metrics_calculator.go - Implement calculations
backend/handlers/analytics.go - Add metrics endpoints
Branch: feature/asset-performance-metrics

PR Title: Add asset performance metrics and ROI tracking

Additional Info: Implement background job for daily metric calculations, add benchmark comparisons, support custom date ranges.

#170 Create user referral system with rewards
Repo Avatar
parkerwinner/kor-AssetForge
Description: Build referral program allowing users to refer new users and earn rewards. Track referral chains, calculate commissions, and manage reward distribution.

File Changes:

backend/models/referral.go - Create referral models
backend/handlers/referral.go - Add referral endpoints
backend/services/referral_service.go - Implement referral logic
backend/migrations/sql/0024_create_referrals.up.sql - Create referral tables
Branch: feature/referral-program

PR Title: Implement user referral program with reward tracking

Additional Info: Add referral code generation, implement tiered rewards, add fraud detection for referral abuse.

# Fixes

github-advanced-security AI found potential problems 1 minute ago
backend/handlers/analytics.go
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date (use YYYY-MM-DD)"})
return
}
metric, err := h.MetricsCalculator.Calculate(uint(assetID), from, to)
github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.

Show more details

@zachyo Reply...
Dismissing the alert will mark this conversation as resolved.
backend/handlers/analytics.go
return
}
limit, \_ := strconv.Atoi(c.DefaultQuery("limit", "90"))
metrics, err := h.MetricsCalculator.History(uint(assetID), limit)
github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.

Show more details

@zachyo Reply...
Dismissing the alert will mark this conversation as resolved.
backend/handlers/analytics.go
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date (use YYYY-MM-DD)"})
return
}
metric, err := h.MetricsCalculator.CalculateAndStore(uint(assetID), from, to)
github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.

Show more details

@zachyo Reply...
Dismissing the alert will mark this conversation as resolved.
backend/handlers/analytics.go
if req.PaidAt != nil {
paidAt = \*req.PaidAt
}
dividend, err := h.MetricsCalculator.RecordDividend(uint(assetID), req.AmountUSD, req.Currency, req.Note, paidAt)
github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.

Show more details

@zachyo Reply...
Dismissing the alert will mark this conversation as resolved.
backend/handlers/comparison.go
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid history id"})
return
}
record, result, err := h.Service.GetHistory(userID, uint(historyID))
github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.

Show more details

@zachyo Reply...
Dismissing the alert will mark this conversation as resolved.
backend/handlers/email_templates.go
}

    variant := models.EmailTemplateVariant{
    	TemplateID: uint(templateID),

github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.

Show more details

@zachyo Reply...
Dismissing the alert will mark this conversation as resolved.
backend/handlers/referral.go
c.JSON(http.StatusBadRequest, gin.H{"error": "invalid referee id"})
return
}
referral, err := h.Service.QualifyReferral(uint(refereeID))
github-advanced-security commented 1 minute ago
@github-advanced-security
github-advanced-security
bot
1 minute ago
High
CodeQL / Incorrect conversion between integer types
Incorrect conversion of an unsigned 64-bit integer from strconv.ParseUint to a lower bit size type uint without an upper bound check.
