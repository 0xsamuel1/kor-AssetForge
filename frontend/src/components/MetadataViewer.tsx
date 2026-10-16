"use client";

import Image from "next/image";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

interface NFTAttribute {
  trait_type: string;
  value: string | number;
  display_type?: string;
}

interface NFTMetadata {
  name: string;
  description: string;
  image: string;
  external_url: string;
  attributes: NFTAttribute[];
  properties?: Record<string, unknown>;
}

interface MetadataViewerProps {
  metadata: NFTMetadata;
  metadataUri?: string;
  metadataHash?: string;
  isImmutable?: boolean;
  ipfsCid?: string;
}

export function MetadataViewer({
  metadata,
  metadataUri,
  metadataHash,
  isImmutable,
  ipfsCid,
}: MetadataViewerProps) {
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {metadata.name}
            {isImmutable && (
              <Badge variant="secondary" className="text-xs">
                Immutable
              </Badge>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {metadata.image && (
            <div className="mb-4">
              <Image
                src={metadata.image}
                alt={metadata.name}
                width={640}
                height={256}
                sizes="(max-width: 768px) 100vw, 640px"
                className="w-full max-h-64 object-cover rounded-lg"
                onError={(e) => {
                  (e.target as HTMLImageElement).src =
                    "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 200'><rect width='200' height='200' fill='%23eee'/><text x='50%25' y='50%25' fill='%23999' text-anchor='middle' dy='.3em'>No Image</text></svg>";
                }}
              />
            </div>
          )}
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">{metadata.description}</p>
            {metadataUri && (
              <p className="text-xs text-muted-foreground">
                Metadata URI:{" "}
                <a
                  href={metadataUri}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary hover:underline"
                >
                  {metadataUri.length > 50
                    ? metadataUri.slice(0, 50) + "..."
                    : metadataUri}
                </a>
              </p>
            )}
            {metadataHash && (
              <p className="text-xs text-muted-foreground">
                Content Hash:{" "}
                <code className="bg-muted px-1 rounded">
                  {metadataHash.slice(0, 16)}...
                </code>
              </p>
            )}
            {ipfsCid && (
              <p className="text-xs text-muted-foreground">
                IPFS CID:{" "}
                <code className="bg-muted px-1 rounded">{ipfsCid}</code>
              </p>
            )}
          </div>
        </CardContent>
      </Card>

      {metadata.attributes && metadata.attributes.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Attributes</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
              {metadata.attributes.map((attr, index) => (
                <div
                  key={index}
                  className="border rounded-lg p-3 text-center"
                >
                  <p className="text-xs text-muted-foreground uppercase tracking-wide mb-1">
                    {attr.trait_type}
                  </p>
                  <p className="text-sm font-medium">{String(attr.value)}</p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {metadata.external_url && (
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">External Link</CardTitle>
          </CardHeader>
          <CardContent>
            <a
              href={metadata.external_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-primary hover:underline text-sm"
            >
              {metadata.external_url}
            </a>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
