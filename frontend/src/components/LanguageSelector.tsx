"use client";

import React from 'react';
import { useLocale } from 'next-intl';
import { useRouter } from 'next/navigation';

const LanguageSelector: React.FC = () => {
  const locale = useLocale();
  const router = useRouter();

  const languages = [
    { code: 'en', name: 'English' },
    { code: 'es', name: 'Español' },
    { code: 'fr', name: 'Français' },
    { code: 'zh', name: '中文' },
  ];

  const handleLanguageChange = (languageCode: string) => {
    document.cookie = `NEXT_LOCALE=${languageCode}; path=/; max-age=31536000; SameSite=Lax`;
    router.refresh();
  };

  return (
    <div className="language-selector">
      <select
        value={locale}
        onChange={(e) => handleLanguageChange(e.target.value)}
        className="language-select border rounded p-1 text-sm bg-background text-foreground"
      >
        {languages.map((lang) => (
          <option key={lang.code} value={lang.code}>
            {lang.name}
          </option>
        ))}
      </select>
    </div>
  );
};

export default LanguageSelector;
