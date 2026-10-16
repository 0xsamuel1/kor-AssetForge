'use client'

import { SearchAutocomplete } from "@/components/SearchAutocomplete";

interface SearchBarProps {
  onSearch: (query: string) => void;
  className?: string;
  initialValue?: string;
}

// Compatibility wrapper for existing consumers. New screens can import
// SearchAutocomplete directly.
export function SearchBar(props: SearchBarProps) {
  return <SearchAutocomplete {...props} />;
}
