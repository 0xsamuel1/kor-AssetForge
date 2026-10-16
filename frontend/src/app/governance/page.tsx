"use client";

import { useState, useEffect, useCallback } from "react";
import { Header } from "@/components/Header";
import { WalletConnect } from "@/components/WalletConnect";
import { ProposalList, Proposal } from "@/components/governance/ProposalList";
import { ProposalForm } from "@/components/governance/ProposalForm";
import { VotePanel } from "@/components/governance/VotePanel";
import { DelegatePanel } from "@/components/governance/DelegatePanel";
import { StellarWallet } from "@/lib/stellar";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PlusCircle, Vote, Users } from "lucide-react";
import { ListSkeleton } from "@/components/LoadingSkeleton";

export default function GovernancePage() {
  const [wallet, setWallet] = useState<StellarWallet | undefined>();
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [selected, setSelected] = useState<Proposal | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [delegateInfo, setDelegateInfo] = useState<{
    currentDelegate?: string;
    delegatedPower: number;
    delegators: string[];
  }>({ delegatedPower: 0, delegators: [] });

  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  const loadProposals = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await fetch(`${api}/api/governance/proposals`);
      if (res.ok) setProposals(await res.json());
    } catch {
      // network error — keep previous state
    } finally {
      setIsLoading(false);
    }
  }, [api]);

  const loadDelegateInfo = useCallback(async () => {
    if (!wallet) return;
    try {
      const res = await fetch(`${api}/api/governance/delegate/${wallet.publicKey}`);
      if (res.ok) setDelegateInfo(await res.json());
    } catch {
      // ignore
    }
  }, [wallet, api]);

  useEffect(() => {
    loadProposals();
  }, [loadProposals]);

  useEffect(() => {
    loadDelegateInfo();
  }, [loadDelegateInfo]);

  // Refresh selected proposal after voting
  const handleVoted = () => {
    loadProposals();
    if (selected) {
      const updated = proposals.find((p) => p.proposal_id === selected.proposal_id);
      if (updated) setSelected(updated);
    }
  };

  return (
    <div className="min-h-screen bg-background">
      <Header wallet={wallet} />
      <main className="container mx-auto px-4 py-8 max-w-5xl">
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-3xl font-bold">Governance</h1>
            <p className="text-muted-foreground mt-1">
              Create proposals, vote on asset governance decisions, and delegate your voting power.
            </p>
          </div>
          {!wallet && (
            <WalletConnect
              onWalletConnected={(w) => setWallet(w)}
              onWalletDisconnected={() => setWallet(undefined)}
              wallet={wallet}
            />
          )}
        </div>

        <Tabs defaultValue="proposals">
          <TabsList className="mb-6">
            <TabsTrigger value="proposals" className="flex items-center gap-2">
              <Vote className="h-4 w-4" />
              Proposals
            </TabsTrigger>
            {wallet && (
              <>
                <TabsTrigger value="create" className="flex items-center gap-2">
                  <PlusCircle className="h-4 w-4" />
                  Create
                </TabsTrigger>
                <TabsTrigger value="delegate" className="flex items-center gap-2">
                  <Users className="h-4 w-4" />
                  Delegate
                </TabsTrigger>
              </>
            )}
          </TabsList>

          <TabsContent value="proposals">
            <div className="grid gap-6 lg:grid-cols-2">
              <div>
                {isLoading ? (
                  <ListSkeleton rows={3} label="Loading proposals" />
                ) : (
                  <ProposalList
                    proposals={proposals}
                    wallet={wallet}
                    onSelect={(p) => setSelected(p)}
                  />
                )}
              </div>
              <div>
                {selected ? (
                  <VotePanel
                    proposal={selected}
                    wallet={wallet}
                    onVoted={handleVoted}
                  />
                ) : (
                  <div className="text-center py-16 text-muted-foreground text-sm">
                    Select a proposal to view details and vote.
                  </div>
                )}
              </div>
            </div>
          </TabsContent>

          {wallet && (
            <>
              <TabsContent value="create">
                <div className="max-w-lg">
                  <ProposalForm wallet={wallet} onCreated={loadProposals} />
                </div>
              </TabsContent>
              <TabsContent value="delegate">
                <div className="max-w-lg">
                  <DelegatePanel
                    wallet={wallet}
                    currentDelegate={delegateInfo.currentDelegate}
                    delegatedPower={delegateInfo.delegatedPower}
                    delegators={delegateInfo.delegators}
                    onUpdated={loadDelegateInfo}
                  />
                </div>
              </TabsContent>
            </>
          )}
        </Tabs>
      </main>
    </div>
  );
}