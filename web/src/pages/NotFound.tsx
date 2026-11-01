import { Link } from "react-router-dom";
import { PageTitle } from "../components/PageTitle";
import { usePageTitle } from "../shell/usePageTitle";

export default function NotFound() {
  usePageTitle("Page not found");
  return (
    <>
      <PageTitle>Page not found</PageTitle>
      <Link to="/" className="btn btn-text">
        Go to the inbox
      </Link>
    </>
  );
}
