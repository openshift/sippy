package html

// 1 encoded job name
// 2 indent depth
// 3 test name
// 4 job name regex
// 5 encoded test name
// 6 bug list/bug search
// 7 pass rate
// 8 number of runs
const testGroupTemplate = `
		<tr class="collapse %s">
			<td colspan=2 style="padding-left:%dpx">
			%s
			<p>
			<a target="_blank" href="https://search.ci.openshift.org/?maxAge=168h&context=1&type=junit&maxMatches=5&maxBytes=20971520&groupBy=job&name=%[4]s&search=%[5]s">Job Search</a>
			%s
			</td>
			<td>%0.2f%% <span class="text-nowrap">(%d runs)</span></td>
		</tr>
	`
