import React from 'react';

export const metadata = {
  title: 'Admin Panel - KOR AssetForge',
  description: 'Platform management and monitoring dashboard',
};

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="bg-gray-50">
      {children}
    </div>
  );
}
