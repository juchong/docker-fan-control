import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { logsApi } from '../services/api';
import { Card } from '../components/common/Card';
import { Button } from '../components/common/Button';
import { EventListResponse } from '../types/monitoring';
import { 
  RefreshCw, 
  Download, 
  Trash2, 
  Search,
  ChevronLeft,
  ChevronRight
} from 'lucide-react';

const LEVELS = ['', 'info', 'warning', 'error'];
const CATEGORIES = ['', 'system', 'fan', 'profile', 'temp', 'ipmi', 'auth'];

export function Logs() {
  const queryClient = useQueryClient();
  const [level, setLevel] = useState('');
  const [category, setCategory] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);
  const limit = 50;

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['logs', level, category, search, page],
    queryFn: () => logsApi.list({
      level: level || '',
      category: category || '',
      search: search || '',
      limit,
      offset: page * limit,
    }),
  });

  const clearMutation = useMutation({
    mutationFn: logsApi.clear,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['logs'] });
    },
  });

  const logs = data as EventListResponse | undefined;
  const totalPages = logs ? Math.ceil(logs.total_count / limit) : 0;

  const handleExport = (format: 'json' | 'csv') => {
    const url = logsApi.export(format, {
      level: level || undefined,
      category: category || undefined,
      search: search || undefined,
    } as Record<string, string>);
    window.open(url, '_blank');
  };

  const getLevelBadge = (eventLevel: string) => {
    switch (eventLevel) {
      case 'error':
        return 'badge-error';
      case 'warning':
        return 'badge-warning';
      default:
        return 'badge-info';
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Event Logs</h1>
        <div className="flex items-center gap-2">
          <Button variant="secondary" onClick={() => refetch()} isLoading={isLoading}>
            <RefreshCw className="w-4 h-4 mr-2" />
            Refresh
          </Button>
          <Button variant="secondary" onClick={() => handleExport('csv')}>
            <Download className="w-4 h-4 mr-2" />
            CSV
          </Button>
          <Button variant="secondary" onClick={() => handleExport('json')}>
            <Download className="w-4 h-4 mr-2" />
            JSON
          </Button>
          <Button
            variant="danger"
            onClick={() => {
              if (confirm('Are you sure you want to clear all logs?')) {
                clearMutation.mutate();
              }
            }}
            isLoading={clearMutation.isPending}
          >
            <Trash2 className="w-4 h-4 mr-2" />
            Clear
          </Button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-4 items-center">
        <div className="flex items-center gap-2">
          <label className="text-sm text-slate-400">Level:</label>
          <select
            className="select w-32"
            value={level}
            onChange={(e) => {
              setLevel(e.target.value);
              setPage(0);
            }}
          >
            <option value="">All</option>
            {LEVELS.filter(Boolean).map((l) => (
              <option key={l} value={l}>
                {l.charAt(0).toUpperCase() + l.slice(1)}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-2">
          <label className="text-sm text-slate-400">Category:</label>
          <select
            className="select w-32"
            value={category}
            onChange={(e) => {
              setCategory(e.target.value);
              setPage(0);
            }}
          >
            <option value="">All</option>
            {CATEGORIES.filter(Boolean).map((c) => (
              <option key={c} value={c}>
                {c.charAt(0).toUpperCase() + c.slice(1)}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-2 flex-1 max-w-md">
          <Search className="w-4 h-4 text-slate-400" />
          <input
            type="text"
            className="input"
            placeholder="Search messages..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(0);
            }}
          />
        </div>

        {logs && (
          <span className="text-sm text-slate-400">
            {logs.total_count} total events
          </span>
        )}
      </div>

      {/* Logs Table */}
      <Card>
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th className="w-44">Timestamp</th>
                <th className="w-24">Level</th>
                <th className="w-28">Category</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {logs?.events?.map((event) => (
                <tr key={event.id}>
                  <td className="text-slate-400 whitespace-nowrap">
                    {new Date(event.timestamp).toLocaleString()}
                  </td>
                  <td>
                    <span className={`badge ${getLevelBadge(event.level)}`}>
                      {event.level}
                    </span>
                  </td>
                  <td className="text-slate-400 capitalize">{event.category}</td>
                  <td className="text-slate-300">{event.message}</td>
                </tr>
              ))}
              {(!logs?.events || logs.events.length === 0) && (
                <tr>
                  <td colSpan={4} className="text-center text-slate-400 py-8">
                    No events found
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between mt-4 pt-4 border-t border-slate-700">
            <span className="text-sm text-slate-400">
              Page {page + 1} of {totalPages}
            </span>
            <div className="flex gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setPage(page - 1)}
                disabled={page === 0}
              >
                <ChevronLeft className="w-4 h-4" />
                Previous
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setPage(page + 1)}
                disabled={page >= totalPages - 1}
              >
                Next
                <ChevronRight className="w-4 h-4" />
              </Button>
            </div>
          </div>
        )}
      </Card>
    </div>
  );
}
