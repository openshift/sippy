export class JobsDashboard extends React.Component {
    constructor(props) {
        super(props);
        const urlParams = new URLSearchParams(window.location.search);
        this.state = {
            loaded: false,
            filter: urlParams.get('job') || "",
            jobs: [],
            error: null,
        };
    }

    updateFilter(newFilter) {
        this.setState({filter: newFilter});
        let urlParams = new URLSearchParams(window.location.search);
        urlParams.set('job', newFilter);
        window.history.replaceState({}, document.title, "?" + urlParams.toString());
    }

    componentDidMount() {
        fetch('/api/jobs?release=' + encodeURIComponent(this.props.release))
            .then(response => response.json())
            .then(response => {
                let jobs = response.jobs;
                jobs.sort((a, b) => a.name > b.name ? 1 : a.name < b.name ? -1 : 0);
                this.setState({
                    loaded: true,
                    jobs: jobs,
                });
            })
            .catch(error => {
                this.setState({
                    loaded: true,
                    error: error.toString(),
                });
            });
    }

    render() {
        if (!this.state.loaded) {
            return 'Loading...';
        }
        if (this.state.error) {
            return 'Failed to load data: ' + this.state.error;
        }

        let filter = new RegExp(this.state.filter);

        let timestampBegin = 0;
        let timestampEnd = 0;
        for (let job of this.state.jobs) {
            if (!filter.test(job.name)) {
                continue;
            }
            for (let ts of  job.timestamps) {
                if (timestampBegin == 0 || ts < timestampBegin) {
                    timestampBegin = ts;
                }
                if (timestampEnd == 0 || ts > timestampEnd) {
                    timestampEnd = ts;
                }
            }
        }

        const msPerDay = 86400*1000;
        timestampBegin = Math.floor(timestampBegin/msPerDay)*msPerDay;
        timestampEnd = Math.floor(timestampEnd/msPerDay)*msPerDay;

        let periodBegin = new Date(timestampBegin);
        let periodEnd = new Date(timestampEnd);

        let header = [
            <td key="name" className="col-header col-name" key="name">
                <input value={this.state.filter} placeholder="regular expression to filter jobs" onChange={(e) => this.updateFilter(e.target.value)} />
            </td>
        ];
        let ts = timestampEnd;
        while (ts >= timestampBegin) {
            let d = new Date(ts);
            let value = (d.getUTCMonth() + 1) + '/' + d.getUTCDate();
            header.push(<td key={"ts-"+ts} className="col-header col-day">{value}</td>);
            ts -= msPerDay;
        }

        let rows = [];
        for (let job of this.state.jobs) {
            if (!filter.test(job.name)) {
                continue;
            }
            let row = [<td key="name" className="col-name" key="name"><a href={job.testgrid_url} target="_blank">{job.name}</a></td>];
            let ts = timestampEnd;
            let i = 0;
            while (ts >= timestampBegin) {
                let results = [];
                while (job.timestamps[i] >= ts) {
                    results.push(<a key={"result-"+i} className={'result result-' + job.results[i]} href={'https://prow.ci.openshift.org/view/gs/origin-ci-test/logs/' + job.name + '/' + job.build_ids[i]} target="_blank">{job.results[i]}</a>);
                    i++;
                }
                row.push(<td key={"ts-"+ts} className="col-day"><div className="results">{results}</div></td>);
                ts -= msPerDay;
            }

            rows.push(<tr key={"job-" + job.name}>{row}</tr>);
        }

        return (
            <div>
                <div>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-S">S</span></span> success</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-F">F</span></span> failure (e2e tests)</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-f">f</span></span> failure (other tests)</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-U">U</span></span> upgrade failure</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-I">I</span></span> setup failure (installer)</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-N">N</span></span> setup failure (infra)</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-n">n</span></span> failure before setup (infra)</span>
                    <span className="legend-item"><span className="results results-demo"><span className="result result-R">R</span></span> running</span>
                </div>
                <table>
                    <thead>
                        <tr>{header}</tr>
                    </thead>
                    <tbody>
                        {rows}
                    </tbody>
                </table>
            </div>
        );
    }
}
