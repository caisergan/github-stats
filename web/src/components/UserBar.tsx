import type { Me } from "../api";

interface Props {
  me: Me;
}

export default function UserBar({ me }: Props) {
  return (
    <header className="user-bar">
      <span className="brand">GitHub Stats</span>
      <span className="who">
        {me.avatar_url && <img src={me.avatar_url} alt="" />}
        <strong>{me.login}</strong>
        <a href="/auth/logout">Sign out</a>
      </span>
    </header>
  );
}
