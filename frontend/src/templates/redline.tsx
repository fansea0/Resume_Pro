import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

function SectionTitle({ children }: { children: string }) {
  return <h2 className="template-redline__section-title">{children}</h2>;
}

function DateLocation({ startDate, endDate, location }: { startDate: string; endDate: string; location: string }) {
  return <span className="template-redline__meta">{[startDate, endDate].filter(Boolean).join(' — ')}{location && ` · ${location}`}</span>;
}

export function RedlineTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  const initial = profile.fullName.trim().slice(0, 1) || '简';

  return (
    <article className="resume-template template-redline">
      <header className="template-redline__header">
        <div>
          <h1>{profile.fullName}</h1>
          {contact && <p className="template-redline__contact">{contact}</p>}
          <p className="template-redline__headline">{profile.headline}</p>
        </div>
        <div aria-label="候选人头像占位" className="template-redline__avatar">{initial}</div>
      </header>

      {profile.summary && <section><SectionTitle>个人总结</SectionTitle><p className="template-redline__summary">{profile.summary}</p></section>}

      {experiences.length > 0 && <section>
        <SectionTitle>工作经历</SectionTitle>
        {experiences.map((experience) => <div className="template-redline__entry" key={experience.id}>
          <div className="template-redline__entry-heading"><h3>{experience.company}</h3><DateLocation startDate={experience.startDate} endDate={experience.endDate} location={profile.location} /></div>
          <p className="template-redline__role">{experience.role}</p>
          {experience.highlights.length > 0 && <ul>{experience.highlights.map((highlight, index) => <li key={index}>{highlight}</li>)}</ul>}
        </div>)}
      </section>}

      {projects.length > 0 && <section>
        <SectionTitle>项目经历</SectionTitle>
        {projects.map((project) => <div className="template-redline__entry" key={project.id}>
          <div className="template-redline__entry-heading"><h3>{project.name}</h3><DateLocation startDate={project.startDate} endDate={project.endDate} location="" /></div>
          <p className="template-redline__role">{[project.role, project.link].filter(Boolean).join(' · ')}</p>
          {project.description && <p>{project.description}</p>}
          {project.highlights.length > 0 && <ul>{project.highlights.map((highlight, index) => <li key={index}>{highlight}</li>)}</ul>}
        </div>)}
      </section>}

      {education.length > 0 && <section>
        <SectionTitle>教育经历</SectionTitle>
        {education.map((item) => <div className="template-redline__entry" key={item.id}>
          <div className="template-redline__entry-heading"><h3>{item.school}</h3><DateLocation startDate={item.startDate} endDate={item.endDate} location="" /></div>
          <p className="template-redline__role">{item.degree}{item.field && ` · ${item.field}`}</p>
        </div>)}
      </section>}

      {skills.length > 0 && <section><SectionTitle>专业技能</SectionTitle><div className="template-redline__skills">{skills.map((skill) => <span key={skill.id}>{skill.name}{skill.level && <small>{skill.level}</small>}</span>)}</div></section>}
    </article>
  );
}
