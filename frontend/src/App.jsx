import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { ThemeProvider } from "@/contexts/ThemeContext";
import { AuthProvider } from "@/contexts/AuthContext";
import { AppLayout } from "@/components/layout";
import {
  StoragePage,
  ComputePage,
  MessageQueuePage,
  DatastorePage,
  HealthPage,
  LogsPage,
  AdminPage,
} from "@/pages";

function App() {
  return (
    <BrowserRouter>
      <ThemeProvider defaultTheme="dark">
        <AuthProvider>
          <Routes>
            <Route element={<AppLayout />}>
              <Route path="/" element={<Navigate to="/storage" replace />} />
              <Route path="/storage" element={<StoragePage />} />
              <Route path="/compute" element={<Navigate to="/compute/containers" replace />} />
              <Route path="/compute/containers" element={<ComputePage view="containers" />} />
              <Route path="/compute/containers/new" element={<ComputePage view="create" />} />
              <Route path="/compute/containers/:containerId" element={<ComputePage view="detail" />} />
              <Route path="/compute/ssh-keys" element={<ComputePage view="ssh-keys" />} />
              <Route path="/message-queue" element={<MessageQueuePage />} />
              <Route path="/datastore" element={<DatastorePage />} />
              <Route path="/health" element={<HealthPage />} />
              <Route path="/logs" element={<LogsPage />} />
              <Route path="/admin" element={<AdminPage />} />
            </Route>
          </Routes>
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}

export default App;
