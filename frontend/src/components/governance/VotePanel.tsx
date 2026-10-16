"use client";

import { useState } from "react";
import { Proposal } from "./ProposalList";
import { StellarWallet } from "@/lib/stellar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import { toast } from "@/lib/toast";
import { ThumbsUp, ThumbsDown, CheckCircle, XCircle, Clock } from "lucide-react";

interface VotePanelProps {
  proposal: Proposal;
  wallet?: StellarWallet;
  onVoted: () => void;
}

export function VotePanel({ proposal, wallet, onVoted }: VotePanelProps) {
  const [isVoting, setIsVoting] = useState(false);

  const total = proposal.votes_for + proposal.votes_against;
  const forPct = total > 0 ? Math.round((proposal.votes_for / total) * 100) : 50;
  const quorumPct = proposal.quorum_threshold > 0
    ? Math.min(100, Math.round((total / proposal.quorum_threshold) * 100))
    : 100;
  const timeLeft = Math.max(0, proposal.end_time - Math.floor(Date.now() / 1000));
  const hours = Math.floor(timeLeft / 3600);
  const minutes = Math.floor((timeLeft % 3600) / 60);

  const castVote = async (support: boolean) => {
    if (!wallet) {
      toast.error("Connect your wallet to vote.");
      return;
    }
    setIsVoting(true);
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"}/api/governance/proposals/${proposal.proposal_id}/vote`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ voter: wallet.publicKey, support }),
        }
      );
      if (!res.ok) throw new Error(await res.text());
      toast.success(`Vote cast: ${support ? "For" : "Against"}`);
      onVoted();
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : "Vote failed.");
    } finally {
      setIsVoting(false);
    }
  };

  const statusIcons = {
    Active: <Clock className="h-4 w-4 text-blue-500" />,
    Passed: <CheckCircle className="h-4 w-4 text-green-500" />,
    Rejected: <XCircle className="h-4 w-4 text-red-500" />,
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between">
          <div>
            <CardTitle>Proposal #{proposal.proposal_id}</CardTitle>
            <CardDescription>Asset ID: {proposal.asset_id}</CardDescription>
          </div>
          <Badge className="flex items-center gap-1">
            {statusIcons[proposal.status]}
            {proposal.status}
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        <p className="text-sm leading-relaxed">{proposal.description}</p>

        <Separator />

        <div className="space-y-3">
          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="text-green-600 font-medium">For — {forPct}%</span>
              <span className="text-red-600 font-medium">Against — {100 - forPct}%</span>
            </div>
            <Progress value={forPct} className="h-3" />
            <div className="flex justify-between text-xs text-muted-foreground mt-1">
              <span>{proposal.votes_for.toLocaleString()} votes</span>
              <span>{proposal.votes_against.toLocaleString()} votes</span>
            </div>
          </div>

          <div>
            <div className="flex justify-between text-sm mb-1">
              <span className="font-medium">Quorum</span>
              <span className="text-muted-foreground">{total.toLocaleString()} / {proposal.quorum_threshold.toLocaleString()}</span>
            </div>
            <Progress value={quorumPct} className="h-2" />
          </div>
        </div>

        {proposal.status === "Active" && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Clock className="h-4 w-4" />
            <span>{hours}h {minutes}m remaining</span>
          </div>
        )}
      </CardContent>

      {proposal.status === "Active" && (
        <CardFooter className="flex gap-3">
          <Button
            variant="outline"
            className="flex-1 border-green-500 text-green-600 hover:bg-green-50"
            disabled={isVoting || !wallet}
            onClick={() => castVote(true)}
          >
            <ThumbsUp className="h-4 w-4 mr-2" />
            Vote For
          </Button>
          <Button
            variant="outline"
            className="flex-1 border-red-500 text-red-600 hover:bg-red-50"
            disabled={isVoting || !wallet}
            onClick={() => castVote(false)}
          >
            <ThumbsDown className="h-4 w-4 mr-2" />
            Vote Against
          </Button>
        </CardFooter>
      )}
    </Card>
  );
}