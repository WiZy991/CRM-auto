import { QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';

import { AuthProvider } from '@/features/auth/AuthProvider';
import { RequireAuth, RequireGuest, RequireRole } from '@/features/auth/guards';
import { CabinetLayout } from '@/layouts/CabinetLayout';
import { PublicLayout } from '@/layouts/PublicLayout';
import { createQueryClient } from '@/lib/query';
import { LoginPage } from '@/pages/auth/LoginPage';
import { RegisterPage } from '@/pages/auth/RegisterPage';
import { AdminAuditPage, AdminModerationPage, AdminUsersPage } from '@/pages/cabinet/AdminPages';
import { BannersPage } from '@/pages/cabinet/BannersPage';
import { CabinetHomePage } from '@/pages/cabinet/CabinetHomePage';
import { DealDetailPage } from '@/pages/cabinet/DealDetailPage';
import { FavoritesPage } from '@/pages/cabinet/FavoritesPage';
import { MyCarsPage } from '@/pages/cabinet/MyCarsPage';
import { ProfilePage } from '@/pages/cabinet/ProfilePage';
import { RequestsPage } from '@/pages/cabinet/RequestsPage';
import { SellerCardPage, SellersCabinetPage } from '@/pages/cabinet/SellersCabinetPage';
import { ForbiddenPage, NotFoundPage } from '@/pages/errors/StatusPages';
import { CatalogPage } from '@/pages/public/CatalogPage';
import { CarPage } from '@/pages/public/CarPage';
import { HomePage } from '@/pages/public/HomePage';
import { AnalyticsLayout } from '@/pages/cabinet/AnalyticsLayout';
import { AnalyticsPage } from '@/pages/cabinet/AnalyticsPage';
import { ReportsPage } from '@/pages/cabinet/ReportsPage';
import { ReportViewPage } from '@/pages/cabinet/ReportViewPage';
import { HelpPage } from '@/pages/cabinet/HelpPage';
import { ChannelsPage } from '@/pages/cabinet/ChannelsPage';
import { DealersPage, DealerPublicPage, HowItWorksPage, SellerPublicPage, SellersPublicPage } from '@/pages/public/InfoPages';
import { ToastProvider } from '@/ui';

const queryClient = createQueryClient();

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <ToastProvider>
            <Routes>
              <Route element={<PublicLayout />}>
                <Route index element={<HomePage />} />
                <Route path="catalog" element={<CatalogPage />} />
                <Route path="catalog/:id" element={<CarPage />} />
                <Route path="dealers" element={<DealersPage />} />
                <Route path="dealers/:slug" element={<DealerPublicPage />} />
                <Route path="sellers" element={<SellersPublicPage />} />
                <Route path="sellers/:id" element={<SellerPublicPage />} />
                <Route path="how-it-works" element={<HowItWorksPage />} />
                <Route path="admin" element={<Navigate to="/app/moderation" replace />} />
                <Route path="403" element={<ForbiddenPage />} />

                <Route element={<RequireGuest />}>
                  <Route path="login" element={<LoginPage />} />
                  <Route path="register" element={<RegisterPage />} />
                </Route>

                <Route path="*" element={<NotFoundPage />} />
              </Route>

              <Route element={<RequireAuth />}>
                <Route path="app" element={<CabinetLayout />}>
                  <Route index element={<CabinetHomePage />} />
                  <Route path="verify" element={<Navigate to="/app" replace />} />
                  <Route path="profile" element={<ProfilePage />} />
                  <Route path="help" element={<HelpPage />} />
                  <Route path="requests" element={<RequestsPage />} />
                  <Route path="deals/:id" element={<DealDetailPage />} />
                  <Route path="favorites" element={<FavoritesPage />} />
                  <Route element={<RequireRole roles={['dealer', 'seller', 'admin']} />}>
                    <Route path="sellers/:id" element={<SellerCardPage />} />
                  </Route>
                  <Route element={<RequireRole roles={['dealer', 'admin']} />}>
                    <Route path="cars" element={<MyCarsPage />} />
                    <Route path="channels" element={<ChannelsPage />} />
                    <Route path="sellers" element={<SellersCabinetPage />} />
                    <Route path="banners" element={<BannersPage />} />
                    <Route path="analytics" element={<AnalyticsLayout />}>
                      <Route index element={<AnalyticsPage />} />
                      <Route path="reports" element={<ReportsPage />} />
                      <Route path="reports/:kind" element={<ReportViewPage />} />
                    </Route>
                  </Route>
                  <Route element={<RequireRole roles={['admin']} />}>
                    <Route path="users" element={<AdminUsersPage />} />
                    <Route path="moderation" element={<AdminModerationPage />} />
                    <Route path="audit" element={<AdminAuditPage />} />
                  </Route>
                  <Route path="*" element={<Navigate to="/app" replace />} />
                </Route>
              </Route>
            </Routes>
          </ToastProvider>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
