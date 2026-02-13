import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles/globals.css";

window.BUILD_INFO = window.BUILD_INFO || { commit: "dev", time: "" };

ReactDOM.createRoot(document.getElementById("root")!).render(<App />);
