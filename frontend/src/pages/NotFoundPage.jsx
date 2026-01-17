import { Link } from "react-router-dom";
import { Header } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Home } from "lucide-react";

export function NotFoundPage() {
  return (
    <div>
      <Header
        eyebrow="Error"
        title="404 - Page Not Found"
        description="The page you're looking for doesn't exist or has been moved."
      />
      <div className="mt-6">
        <Button asChild>
          <Link to="/">
            <Home className="w-4 h-4 mr-2" />
            Go Home
          </Link>
        </Button>
      </div>
    </div>
  );
}
