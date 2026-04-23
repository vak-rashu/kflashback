import { useState, useEffect, useRef } from 'react';
import { Sparkles, Send, AlertTriangle, Shield, MessageCircle, Clock, ChevronDown } from 'lucide-react';
import { askAI, getAnomalies } from '../api/client';
import type { Anomaly, AnomalyReport } from '../types';

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
}

export default function AIInsights() {
  const [activeTab, setActiveTab] = useState<'chat' | 'anomalies'>('chat');
  const [messages, setMessages] = useState<ChatMessage[]>([{
    role: 'assistant',
    content: 'I can answer questions about your Kubernetes cluster resources and changes. Try:\n\n- "How many deployments are tracked?"\n- "What namespaces have resources?"\n- "What changed in the last hour?"\n- "Which resources were deleted recently?"\n\nNote: I can only answer Kubernetes-related questions. Off-topic questions will be declined.',
    timestamp: new Date(),
  }]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [anomalyReport, setAnomalyReport] = useState<AnomalyReport | null>(null);
  const [anomalyLoading, setAnomalyLoading] = useState(false);
  const [anomalyHours, setAnomalyHours] = useState(24);
  const chatEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = async () => {
    const question = input.trim();
    if (!question || loading) return;

    setInput('');
    setMessages(prev => [...prev, { role: 'user', content: question, timestamp: new Date() }]);
    setLoading(true);

    try {
      const result = await askAI(question);
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: result.answer,
        timestamp: new Date(),
      }]);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Something went wrong';
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: `Sorry, I couldn't process that: ${msg}`,
        timestamp: new Date(),
      }]);
    } finally {
      setLoading(false);
    }
  };

  const loadAnomalies = async () => {
    setAnomalyLoading(true);
    try {
      const report = await getAnomalies(anomalyHours);
      setAnomalyReport(report);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to detect anomalies';
      setAnomalyReport({ anomalies: [], summary: `Error: ${msg}` });
    } finally {
      setAnomalyLoading(false);
    }
  };

  // Reset report when time window changes so user can re-scan
  useEffect(() => {
    setAnomalyReport(null);
  }, [anomalyHours]);

  const severityColor = (s: string) => {
    switch (s) {
      case 'high': return 'bg-red-100 text-red-800 border-red-200';
      case 'medium': return 'bg-yellow-100 text-yellow-800 border-yellow-200';
      case 'low': return 'bg-blue-100 text-blue-800 border-blue-200';
      default: return 'bg-gray-100 text-gray-800 border-gray-200';
    }
  };

  const severityIcon = (s: string) => {
    switch (s) {
      case 'high': return <AlertTriangle className="w-4 h-4 text-red-600" />;
      case 'medium': return <AlertTriangle className="w-4 h-4 text-yellow-600" />;
      default: return <Shield className="w-4 h-4 text-blue-600" />;
    }
  };

  return (
    <div className="max-w-6xl mx-auto">
      <div className="flex items-center gap-2 mb-4">
        <Sparkles className="w-5 h-5 text-purple-600" />
        <h1 className="text-lg font-bold text-gray-900">AI Insights</h1>
        <span className="text-xs bg-purple-100 text-purple-700 px-2 py-0.5 rounded-full font-medium">Beta</span>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-4 bg-gray-100 p-0.5 rounded-lg w-fit">
        <button
          onClick={() => setActiveTab('chat')}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
            activeTab === 'chat' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          <MessageCircle className="w-3.5 h-3.5" />
          Ask AI
        </button>
        <button
          onClick={() => setActiveTab('anomalies')}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
            activeTab === 'anomalies' ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-600 hover:text-gray-900'
          }`}
        >
          <AlertTriangle className="w-3.5 h-3.5" />
          Anomaly Detection
          {anomalyReport && (anomalyReport.anomalies?.length ?? 0) > 0 && (
            <span className="bg-red-500 text-white text-xs px-1.5 py-0.5 rounded-full min-w-[18px] text-center">
              {anomalyReport.anomalies.length}
            </span>
          )}
        </button>
      </div>

      {/* Chat Tab */}
      {activeTab === 'chat' && (
        <div className="bg-white rounded-lg border border-gray-200 flex flex-col" style={{ height: 'calc(100vh - 200px)' }}>
          {/* Messages */}
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {messages.map((msg, i) => (
              <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                <div className={`max-w-[75%] rounded-lg px-3 py-2 text-sm ${
                  msg.role === 'user'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-100 text-gray-800'
                }`}>
                  <div className="whitespace-pre-wrap">{msg.content}</div>
                  <div className={`text-xs mt-1 ${msg.role === 'user' ? 'text-blue-200' : 'text-gray-400'}`}>
                    {msg.timestamp.toLocaleTimeString()}
                  </div>
                </div>
              </div>
            ))}
            {loading && (
              <div className="flex justify-start">
                <div className="bg-gray-100 rounded-lg px-3 py-2 text-sm text-gray-500">
                  <div className="flex items-center gap-2">
                    <div className="flex gap-1">
                      <div className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                      <div className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                      <div className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
                    </div>
                    Analyzing cluster data...
                  </div>
                </div>
              </div>
            )}
            <div ref={chatEndRef} />
          </div>

          {/* Input */}
          <div className="border-t border-gray-200 p-3">
            <div className="flex gap-2">
              <input
                type="text"
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSend()}
                placeholder="Ask about your cluster changes..."
                className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                disabled={loading}
              />
              <button
                onClick={handleSend}
                disabled={loading || !input.trim()}
                className="px-3 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              >
                <Send className="w-4 h-4" />
              </button>
            </div>
            <p className="text-xs text-gray-400 mt-1.5">
              AI responses are based on tracked resource data. Sensitive information is automatically redacted.
            </p>
          </div>
        </div>
      )}

      {/* Anomalies Tab */}
      {activeTab === 'anomalies' && (
        <div className="space-y-3">
          {/* Controls */}
          <div className="flex items-center justify-between bg-white rounded-lg border border-gray-200 px-4 py-2.5">
            <div className="flex items-center gap-2">
              <Clock className="w-4 h-4 text-gray-500" />
              <span className="text-sm text-gray-600">Time window:</span>
              <div className="relative">
                <select
                  value={anomalyHours}
                  onChange={e => setAnomalyHours(Number(e.target.value))}
                  className="appearance-none bg-gray-50 border border-gray-200 rounded-md px-2.5 py-1 pr-7 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  <option value={1}>Last hour</option>
                  <option value={6}>Last 6 hours</option>
                  <option value={24}>Last 24 hours</option>
                  <option value={72}>Last 3 days</option>
                  <option value={168}>Last 7 days</option>
                </select>
                <ChevronDown className="w-3 h-3 text-gray-400 absolute right-2 top-1/2 -translate-y-1/2 pointer-events-none" />
              </div>
            </div>
            <button
              onClick={loadAnomalies}
              disabled={anomalyLoading}
              className="px-3 py-1.5 text-sm bg-purple-600 text-white rounded-md hover:bg-purple-700 disabled:opacity-50 transition-colors"
            >
              {anomalyLoading ? 'Scanning...' : 'Scan for Anomalies'}
            </button>
          </div>

          {/* Summary */}
          {anomalyReport && (
            <div className={`rounded-lg border px-4 py-3 ${
              (anomalyReport.anomalies?.length ?? 0) === 0
                ? 'bg-green-50 border-green-200'
                : 'bg-orange-50 border-orange-200'
            }`}>
              <p className="text-sm font-medium text-gray-800">{anomalyReport.summary}</p>
            </div>
          )}

          {/* Anomaly Cards */}
          {(anomalyReport?.anomalies ?? []).map((anomaly: Anomaly, i: number) => (
            <div key={i} className={`rounded-lg border p-4 ${severityColor(anomaly.severity)}`}>
              <div className="flex items-start gap-3">
                <div className="mt-0.5">{severityIcon(anomaly.severity)}</div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-1">
                    <h3 className="text-sm font-semibold">{anomaly.title}</h3>
                    <span className={`text-xs px-1.5 py-0.5 rounded font-medium uppercase ${
                      anomaly.severity === 'high' ? 'bg-red-200 text-red-900' :
                      anomaly.severity === 'medium' ? 'bg-yellow-200 text-yellow-900' :
                      'bg-blue-200 text-blue-900'
                    }`}>
                      {anomaly.severity}
                    </span>
                  </div>
                  <p className="text-sm opacity-90">{anomaly.description}</p>
                  {anomaly.resource && (
                    <p className="text-xs mt-1.5 opacity-70">
                      Resource: {anomaly.resource}
                      {anomaly.revision > 0 && ` · Revision ${anomaly.revision}`}
                    </p>
                  )}
                </div>
              </div>
            </div>
          ))}

          {!anomalyReport && !anomalyLoading && (
            <div className="flex flex-col items-center justify-center py-12 text-gray-400">
              <AlertTriangle className="w-10 h-10 mb-2" />
              <p className="text-sm font-medium">Click "Scan for Anomalies" to analyze recent changes</p>
              <p className="text-xs mt-1">The AI will review changes in the selected time window for unusual patterns.</p>
            </div>
          )}

          {anomalyLoading && (
            <div className="flex items-center justify-center py-12">
              <div className="flex items-center gap-3 text-purple-600">
                <Sparkles className="w-5 h-5 animate-pulse" />
                <span className="text-sm font-medium">AI is analyzing recent changes for anomalies...</span>
              </div>
            </div>
          )}

          {anomalyReport && (anomalyReport.anomalies?.length ?? 0) === 0 && !anomalyLoading && (
            <div className="flex flex-col items-center justify-center py-12 text-gray-500">
              <Shield className="w-10 h-10 mb-2 text-green-500" />
              <p className="text-sm font-medium">No anomalies detected</p>
              <p className="text-xs mt-1">Your cluster changes look normal for the selected time window.</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
