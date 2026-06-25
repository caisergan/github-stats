import { I } from "./Icons";
import type {
  LatestItem,
  LatestCommit,
  LatestPR,
  LatestIssue,
  LatestKind,
} from "../api";
import { fmtRelative } from "../format";

interface Props {
  kind: LatestKind;
  items: LatestItem[];
  repoFullName?: string; // when set, rows link to the commit/PR/issue on GitHub
}

function isCommit(i: LatestItem): i is LatestCommit {
  return (i as LatestCommit).sha !== undefined;
}

export default function LatestList({ kind, items, repoFullName }: Props) {
  if (items.length === 0) {
    return <div className="empty">Nothing yet.</div>;
  }

  return (
    <div className="latest">
      {items.map((item, i) => {
        if (isCommit(item)) {
          const inner = (
            <>
              <span className="ic green">
                <I.commit style={{ width: 14, height: 14 }} />
              </span>
              <div className="body">
                <div className="ttl">{item.msg_first_line}</div>
                <div className="sub">
                  <span className="sha">{item.sha.slice(0, 7)}</span>
                  <span>·</span>
                  <span>
                    {item.author_login}
                    {item.is_bot && " 🤖"}
                  </span>
                  <span>·</span>
                  <span>{fmtRelative(item.committed_at)}</span>
                </div>
              </div>
              <span className="churn">
                <span className="add">+{item.additions}</span>{" "}
                <span className="del">−{item.deletions}</span>
              </span>
            </>
          );
          return repoFullName ? (
            <a
              className="item link"
              key={item.sha || i}
              href={`https://github.com/${repoFullName}/commit/${item.sha}`}
              target="_blank"
              rel="noopener noreferrer"
              title="Open commit on GitHub"
            >
              {inner}
            </a>
          ) : (
            <div className="item" key={item.sha || i}>
              {inner}
            </div>
          );
        }

        if (kind === "prs") {
          const pr = item as LatestPR;
          const tone =
            pr.merged_at !== null ? "purple" : pr.closed_at !== null ? "red" : "green";
          const inner = (
            <>
              <span className={"ic " + tone}>
                <I.pr style={{ width: 14, height: 14 }} />
              </span>
              <div className="body">
                <div className="ttl">{pr.title}</div>
                <div className="sub">
                  <span>#{pr.number}</span>
                  <span>·</span>
                  <span>{pr.author_login}</span>
                  <span>·</span>
                  <span>{fmtRelative(pr.created_at)}</span>
                </div>
              </div>
              <span className="row" style={{ gap: 4, color: "var(--muted)", fontSize: 12 }}>
                <I.comment style={{ width: 13, height: 13 }} />
                {pr.comments_count}
              </span>
            </>
          );
          return repoFullName ? (
            <a
              className="item link"
              key={"pr" + (pr.number || i)}
              href={`https://github.com/${repoFullName}/pull/${pr.number}`}
              target="_blank"
              rel="noopener noreferrer"
              title="Open pull request on GitHub"
            >
              {inner}
            </a>
          ) : (
            <div className="item" key={"pr" + (pr.number || i)}>
              {inner}
            </div>
          );
        }

        const iss = item as LatestIssue;
        const tone = iss.closed_at !== null ? "purple" : "green";
        const inner = (
          <>
            <span className={"ic " + tone}>
              <I.issue style={{ width: 14, height: 14 }} />
            </span>
            <div className="body">
              <div className="ttl">{iss.title}</div>
              <div className="sub">
                <span>#{iss.number}</span>
                <span>·</span>
                <span>{iss.author_login}</span>
                <span>·</span>
                <span>opened {fmtRelative(iss.created_at)}</span>
              </div>
            </div>
            <span className="row" style={{ gap: 4, color: "var(--muted)", fontSize: 12 }}>
              <I.comment style={{ width: 13, height: 13 }} />
              {iss.comments_count}
            </span>
          </>
        );
        return repoFullName ? (
          <a
            className="item link"
            key={"is" + (iss.number || i)}
            href={`https://github.com/${repoFullName}/issues/${iss.number}`}
            target="_blank"
            rel="noopener noreferrer"
            title="Open issue on GitHub"
          >
            {inner}
          </a>
        ) : (
          <div className="item" key={"is" + (iss.number || i)}>
            {inner}
          </div>
        );
      })}
    </div>
  );
}
