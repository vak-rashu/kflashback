import { Link, useLocation } from 'react-router-dom';
import { LayoutDashboard, Box, History, Sparkles } from 'lucide-react';

const navItems = [
  { path: '/', label: 'Dashboard', icon: LayoutDashboard },
  { path: '/resources', label: 'Resources', icon: Box },
  { path: '/ai', label: 'AI Insights', icon: Sparkles },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation();

  return (
    <div className="min-h-screen flex">
      {/* Sidebar */}
      <aside className="w-52 bg-gray-900 text-white flex flex-col fixed h-full">
        <div className="px-4 py-3 border-b border-gray-700">
          <Link to="/" className="flex items-center gap-3">
            <div className="w-7 h-7 bg-blue-600 rounded-md flex items-center justify-center">
              <History className="w-4 h-4 text-white" />
            </div>
            <div>
              <h1 className="text-base font-bold tracking-tight">kflashback</h1>
              <p className="text-xs text-gray-400">Resource History</p>
            </div>
          </Link>
        </div>

        <nav className="flex-1 p-2 space-y-0.5">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive =
              item.path === '/'
                ? location.pathname === '/'
                : location.pathname.startsWith(item.path);

            return (
              <Link
                key={item.path}
                to={item.path}
                className={`flex items-center gap-2.5 px-2.5 py-2 rounded-md text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-blue-600 text-white'
                    : 'text-gray-300 hover:bg-gray-800 hover:text-white'
                }`}
              >
                <Icon className="w-4.5 h-4.5" />
                {item.label}
              </Link>
            );
          })}
        </nav>

        <div className="px-3 py-2.5 border-t border-gray-700">
          <div className="text-xs text-gray-500">
            <p>v0.1.0-alpha</p>
            <p className="mt-0.5">CNCF Sandbox Project</p>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 ml-52">
        <div className="px-6 py-5">{children}</div>
      </main>
    </div>
  );
}
