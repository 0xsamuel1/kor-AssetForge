import { useFormatter } from 'next-intl';

export const useLocalization = () => {
  const format = useFormatter();

  const formatDate = (date: Date | string): string => {
    const d = typeof date === 'string' ? new Date(date) : date;
    return format.dateTime(d, {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  };

  const formatTime = (date: Date | string): string => {
    const d = typeof date === 'string' ? new Date(date) : date;
    return format.dateTime(d, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  };

  const formatDateTime = (date: Date | string): string => {
    const d = typeof date === 'string' ? new Date(date) : date;
    return format.dateTime(d, {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  };

  const formatNumber = (value: number, minimumFractionDigits = 0, maximumFractionDigits = 2): string => {
    return format.number(value, {
      minimumFractionDigits,
      maximumFractionDigits,
    });
  };

  const formatCurrency = (value: number, currency = 'USD'): string => {
    return format.number(value, {
      style: 'currency',
      currency,
    });
  };

  const formatPercent = (value: number, minimumFractionDigits = 0): string => {
    return format.number(value, {
      style: 'percent',
      minimumFractionDigits,
    });
  };

  return {
    formatDate,
    formatTime,
    formatDateTime,
    formatNumber,
    formatCurrency,
    formatPercent,
  };
};
