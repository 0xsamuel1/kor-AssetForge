use soroban_sdk::{contract, contractimpl, contracttype, Address, Env, Symbol, String};

// ============================================================================
// Data Types
// ============================================================================

#[contracttype]
#[derive(Clone, Debug, PartialEq)]
pub enum ClaimStatus {
    Pending,
    Approved,
    Rejected,
    Paid,
}

/// Risk factors sourced from oracle/admin (each 0–100, where 100 = highest risk).
#[contracttype]
#[derive(Clone)]
pub struct RiskProfile {
    pub asset_type_risk: u32,
    pub value_risk: u32,
    pub claims_history_risk: u32,
    pub market_volatility: u32,
}

/// Installment-based payment plan for a policy (informational: no on-chain token pull).
/// Policy validity at claim time checks paid_count against required_installments.
#[contracttype]
#[derive(Clone)]
pub struct PaymentPlan {
    pub total_installments: u32,
    pub interval: u64,
    pub amount_per_installment: i128,
    pub paid_count: u32,
    pub next_due: u64,
}

#[contracttype]
#[derive(Clone)]
pub struct InsurancePolicy {
    pub policy_id: u64,
    pub asset_id: u64,
    pub premium_amount: i128,
    pub coverage_amount: i128,
    pub policyholder: Address,
    pub active: bool,
    pub expires_at: u64,
    pub has_payment_plan: bool,
}

#[contracttype]
#[derive(Clone)]
pub struct InsuranceClaim {
    pub claim_id: u64,
    pub policy_id: u64,
    pub claimant: Address,
    pub amount_requested: i128,
    pub status: ClaimStatus,
    pub filed_at: u64,
    pub evidence_hash: String,
}

#[derive(Clone)]
#[contracttype]
pub enum DataKey {
    Admin,
    Policy(u64),
    Claim(u64),
    PolicyCount,
    ClaimCount,
    RiskProfile(u64),   // per asset_id
    PaymentPlan(u64),   // per policy_id
}

// ============================================================================
// Contract
// ============================================================================

#[contract]
pub struct AssetInsurance;

#[contractimpl]
impl AssetInsurance {
    pub fn initialize_insurance(env: Env, admin: Address) {
        if env.storage().instance().has(&DataKey::Admin) {
            panic!("already initialized");
        }
        env.storage().instance().set(&DataKey::Admin, &admin);
        env.storage().instance().set(&DataKey::PolicyCount, &0u64);
        env.storage().instance().set(&DataKey::ClaimCount, &0u64);
    }

    /// Admin: set oracle-sourced risk profile for an asset.
    pub fn set_risk_profile(env: Env, admin: Address, asset_id: u64, profile: RiskProfile) {
        admin.require_auth();
        let expected_admin: Address = env
            .storage()
            .instance()
            .get(&DataKey::Admin)
            .expect("not initialized");
        if admin != expected_admin {
            panic!("admin only");
        }
        if profile.asset_type_risk > 100
            || profile.value_risk > 100
            || profile.claims_history_risk > 100
            || profile.market_volatility > 100
        {
            panic!("risk factors must be 0-100");
        }
        env.storage()
            .persistent()
            .set(&DataKey::RiskProfile(asset_id), &profile);

        env.events().publish(
            (Symbol::new(&env, "risk_profile_set"), asset_id),
            (profile.asset_type_risk, profile.value_risk),
        );
    }

    /// Calculate dynamic premium for a coverage amount and duration.
    /// base_rate = 100 bps (1%). Risk multiplier derived from avg risk factor (0–100).
    /// Long-term discount: ≥1 year → 10% off; ≥2 years → 20% off.
    pub fn calculate_premium(env: Env, asset_id: u64, coverage: i128, duration: u64) -> i128 {
        if coverage <= 0 {
            panic!("coverage must be positive");
        }
        let base_rate_bps: i128 = 100; // 1%

        let risk_multiplier_bps: i128 = if let Some(profile) = env
            .storage()
            .persistent()
            .get::<DataKey, RiskProfile>(&DataKey::RiskProfile(asset_id))
        {
            // Average of all four risk factors (0–100), scaled to 50–200 bps range
            // avg=0 → 50 bps (0.5×), avg=50 → 100 bps (1×), avg=100 → 200 bps (2×)
            let avg = (profile.asset_type_risk as i128
                + profile.value_risk as i128
                + profile.claims_history_risk as i128
                + profile.market_volatility as i128)
                / 4;
            50 + avg * 150 / 100
        } else {
            // No risk profile: use neutral 1× multiplier
            100
        };

        // Long-term discount factor in bps (applied as reduction)
        let seconds_per_year: u64 = 31_536_000;
        let discount_bps: i128 = if duration >= 2 * seconds_per_year {
            20 // 20% off
        } else if duration >= seconds_per_year {
            10 // 10% off
        } else {
            0
        };

        // premium = coverage * base_rate_bps / 10_000 * risk_multiplier_bps / 100
        //           * (100 - discount_bps) / 100
        let raw = coverage * base_rate_bps / 10_000 * risk_multiplier_bps / 100;
        raw * (100 - discount_bps) / 100
    }

    /// Purchase a policy. Premium is auto-calculated based on risk profile.
    /// If use_payment_plan is true, a PaymentPlan record is created (informational).
    pub fn purchase_policy(
        env: Env,
        policyholder: Address,
        asset_id: u64,
        coverage: i128,
        duration: u64,
        use_payment_plan: bool,
        installments: u32,
    ) -> u64 {
        policyholder.require_auth();

        if coverage <= 0 {
            panic!("coverage must be positive");
        }
        if duration == 0 {
            panic!("duration must be positive");
        }

        let premium = Self::calculate_premium(env.clone(), asset_id, coverage, duration);

        let count: u64 = env
            .storage()
            .instance()
            .get(&DataKey::PolicyCount)
            .unwrap_or(0);
        let policy_id = count + 1;

        let now = env.ledger().timestamp();

        let policy = InsurancePolicy {
            policy_id,
            asset_id,
            premium_amount: premium,
            coverage_amount: coverage,
            policyholder: policyholder.clone(),
            active: true,
            expires_at: now + duration,
            has_payment_plan: use_payment_plan && installments > 1,
        };

        env.storage()
            .persistent()
            .set(&DataKey::Policy(policy_id), &policy);
        env.storage()
            .instance()
            .set(&DataKey::PolicyCount, &policy_id);

        if use_payment_plan && installments > 1 {
            let amount_per = premium / installments as i128;
            let interval = duration / installments as u64;
            let plan = PaymentPlan {
                total_installments: installments,
                interval,
                amount_per_installment: amount_per,
                paid_count: 0,
                next_due: now + interval,
            };
            env.storage()
                .persistent()
                .set(&DataKey::PaymentPlan(policy_id), &plan);
        }

        env.events()
            .publish((Symbol::new(&env, "policy_purchased"), policyholder), policy_id);

        policy_id
    }

    /// Mark next installment as paid. Policy remains active while paid_count < total_installments.
    pub fn pay_installment(env: Env, policyholder: Address, policy_id: u64) {
        policyholder.require_auth();

        let policy: InsurancePolicy = env
            .storage()
            .persistent()
            .get(&DataKey::Policy(policy_id))
            .expect("policy not found");

        if policy.policyholder != policyholder {
            panic!("only policyholder can pay installment");
        }
        if !policy.has_payment_plan {
            panic!("policy does not have a payment plan");
        }

        let mut plan: PaymentPlan = env
            .storage()
            .persistent()
            .get(&DataKey::PaymentPlan(policy_id))
            .expect("payment plan not found");

        if plan.paid_count >= plan.total_installments {
            panic!("all installments already paid");
        }

        let now = env.ledger().timestamp();
        plan.paid_count += 1;
        plan.next_due = now + plan.interval;

        env.storage()
            .persistent()
            .set(&DataKey::PaymentPlan(policy_id), &plan);

        env.events().publish(
            (Symbol::new(&env, "installment_paid"), policyholder),
            (policy_id, plan.paid_count),
        );
    }

    /// Renew an expired or soon-to-expire policy with a fresh premium calculation.
    pub fn renew_policy(
        env: Env,
        policyholder: Address,
        policy_id: u64,
        new_duration: u64,
    ) -> u64 {
        policyholder.require_auth();

        if new_duration == 0 {
            panic!("duration must be positive");
        }

        let old_policy: InsurancePolicy = env
            .storage()
            .persistent()
            .get(&DataKey::Policy(policy_id))
            .expect("policy not found");

        if old_policy.policyholder != policyholder {
            panic!("only policyholder can renew");
        }

        // Increment claims history risk on renewal if any claims were filed
        // (admin must update the risk profile separately via set_risk_profile)
        let new_premium = Self::calculate_premium(
            env.clone(),
            old_policy.asset_id,
            old_policy.coverage_amount,
            new_duration,
        );

        let count: u64 = env
            .storage()
            .instance()
            .get(&DataKey::PolicyCount)
            .unwrap_or(0);
        let new_policy_id = count + 1;

        let now = env.ledger().timestamp();

        let new_policy = InsurancePolicy {
            policy_id: new_policy_id,
            asset_id: old_policy.asset_id,
            premium_amount: new_premium,
            coverage_amount: old_policy.coverage_amount,
            policyholder: policyholder.clone(),
            active: true,
            expires_at: now + new_duration,
            has_payment_plan: false,
        };

        env.storage()
            .persistent()
            .set(&DataKey::Policy(new_policy_id), &new_policy);
        env.storage()
            .instance()
            .set(&DataKey::PolicyCount, &new_policy_id);

        // Deactivate old policy
        let mut old = old_policy;
        old.active = false;
        env.storage()
            .persistent()
            .set(&DataKey::Policy(policy_id), &old);

        env.events().publish(
            (Symbol::new(&env, "policy_renewed"), policyholder),
            (policy_id, new_policy_id, new_premium),
        );

        new_policy_id
    }

    pub fn file_claim(
        env: Env,
        claimant: Address,
        policy_id: u64,
        amount: i128,
        evidence_hash: String,
    ) -> u64 {
        claimant.require_auth();

        let policy: InsurancePolicy = env
            .storage()
            .persistent()
            .get(&DataKey::Policy(policy_id))
            .expect("policy not found");

        if policy.policyholder != claimant {
            panic!("only policyholder can file a claim");
        }
        if !policy.active {
            panic!("policy is not active");
        }
        let now = env.ledger().timestamp();
        if now > policy.expires_at {
            panic!("policy has expired");
        }
        if amount > policy.coverage_amount {
            panic!("claim exceeds coverage");
        }

        // Check payment plan compliance
        if policy.has_payment_plan {
            let plan: PaymentPlan = env
                .storage()
                .persistent()
                .get(&DataKey::PaymentPlan(policy_id))
                .expect("payment plan not found");
            if plan.paid_count == 0 {
                panic!("no installments paid");
            }
        }

        let count: u64 = env
            .storage()
            .instance()
            .get(&DataKey::ClaimCount)
            .unwrap_or(0);
        let claim_id = count + 1;

        let claim = InsuranceClaim {
            claim_id,
            policy_id,
            claimant: claimant.clone(),
            amount_requested: amount,
            status: ClaimStatus::Pending,
            filed_at: now,
            evidence_hash,
        };

        env.storage()
            .persistent()
            .set(&DataKey::Claim(claim_id), &claim);
        env.storage()
            .instance()
            .set(&DataKey::ClaimCount, &claim_id);

        env.events()
            .publish((Symbol::new(&env, "claim_filed"), claimant), claim_id);

        claim_id
    }

    pub fn process_claim(env: Env, admin: Address, claim_id: u64, approved: bool) {
        admin.require_auth();
        let expected_admin: Address = env
            .storage()
            .instance()
            .get(&DataKey::Admin)
            .expect("not initialized");
        if admin != expected_admin {
            panic!("admin only");
        }

        let mut claim: InsuranceClaim = env
            .storage()
            .persistent()
            .get(&DataKey::Claim(claim_id))
            .expect("claim not found");

        if claim.status != ClaimStatus::Pending {
            panic!("claim already processed");
        }

        if approved {
            claim.status = ClaimStatus::Approved;
            // Payout logic would happen here in a real implementation
        } else {
            claim.status = ClaimStatus::Rejected;
        }

        env.storage()
            .persistent()
            .set(&DataKey::Claim(claim_id), &claim);

        env.events().publish(
            (Symbol::new(&env, "claim_processed"), claim_id),
            approved,
        );
    }

    pub fn get_policy(env: Env, policy_id: u64) -> Option<InsurancePolicy> {
        env.storage().persistent().get(&DataKey::Policy(policy_id))
    }

    pub fn get_claim(env: Env, claim_id: u64) -> Option<InsuranceClaim> {
        env.storage().persistent().get(&DataKey::Claim(claim_id))
    }

    pub fn get_risk_profile(env: Env, asset_id: u64) -> Option<RiskProfile> {
        env.storage()
            .persistent()
            .get(&DataKey::RiskProfile(asset_id))
    }

    pub fn get_payment_plan(env: Env, policy_id: u64) -> Option<PaymentPlan> {
        env.storage()
            .persistent()
            .get(&DataKey::PaymentPlan(policy_id))
    }
}
