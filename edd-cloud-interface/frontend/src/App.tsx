import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { ThemeProvider } from "@/contexts/ThemeContext";
import { AuthProvider } from "@/contexts/AuthContext";
import { NotificationProvider } from "@/contexts/NotificationContext";
import { AppLayout } from "@/components/layout";
import {
  StoragePage,
  ComputePage,
  MessageQueuePage,
  DatastorePage,
  HealthPage,
  AdminPage,
  ServiceAccountsPage,
  NotFoundPage,
} from "@/pages";

function App() {
  return (
    <BrowserRouter>
      <ThemeProvider defaultTheme="dark">
        <AuthProvider>
          <NotificationProvider>
          <Routes>
            <Route element={<AppLayout />}>
              <Route path="/" element={<Navigate to="/compute" replace />} />
              <Route path="/storage" element={<StoragePage />} />
              <Route path="/storage/:namespace" element={<StoragePage />} />
              <Route path="/compute" element={<Navigate to="/compute/containers" replace />} />
              <Route path="/compute/containers" element={<ComputePage view="containers" />} />
              <Route path="/compute/containers/new" element={<ComputePage view="create" />} />
              <Route path="/compute/containers/:containerId" element={<ComputePage view="detail" />} />
              <Route path="/compute/ssh-keys" element={<ComputePage view="ssh-keys" />} />
              <Route path="/message-queue" element={<MessageQueuePage />} />
              <Route path="/datastore" element={<DatastorePage />} />
              <Route path="/health" element={<HealthPage />} />
              <Route path="/logs" element={<Navigate to="/health#logs" replace />} />
              <Route path="/service-accounts" element={<ServiceAccountsPage />} />
              <Route path="/service-accounts/tokens" element={<ServiceAccountsPage view="tokens" />} />
              <Route path="/service-accounts/:id" element={<ServiceAccountsPage />} />
              <Route path="/auth-token" element={<Navigate to="/service-accounts" replace />} />
              <Route path="/auth-token/tokens" element={<Navigate to="/service-accounts/tokens" replace />} />
              <Route path="/admin" element={<AdminPage />} />
              <Route path="*" element={<NotFoundPage />} />
            </Route>
          </Routes>
          </NotificationProvider>
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}

export default App;
