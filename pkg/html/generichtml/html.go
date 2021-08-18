package generichtml

var (
	HTMLPageStart = `
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8"><title>%s</title>
<link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.1.3/css/bootstrap.min.css" integrity="sha384-MCw98/SFnGE8fJT3GXwEOngsV7Zt27NXFoaoApmYm81iuXoPkFOJwJ8ERdknLPMO" crossorigin="anonymous">
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.min.css">
<meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
<link rel="apple-touch-icon" sizes="180x180" href="/static/apple-touch-icon.png">
<link rel="icon" type="image/png" sizes="32x32" href="/static/favicon-32x32.png">
<link rel="icon" type="image/png" sizes="16x16" href="/static/favicon-16x16.png">
<link rel="manifest" href="/static/site.webmanifest">
<style>
@media (max-width: 992px) {
  .container {
    width: 100%%;
    max-width: none;
  }
}

.error {
	background-color: #f5969b;
}
</style>
</head>

<body>
<div class="container">
<div align="right">
	<p><a href="/sippy-ng">Try out the new UI for Sippy</a></p>
</div>
`

	HTMLPageEnd = `
</div>
Data current as of: %s
<p>
<a href="https://amd64.ocp.releases.ci.openshift.org/dashboards/overview">Release Dashboard</a> |
<a href="https://sippy-historical-bparees.apps.ci.l2s4.p1.openshiftapps.com/">Historical Data</a> |
<a href="https://github.com/openshift/sippy">Source Code</a>
<script src="https://code.jquery.com/jquery-3.2.1.slim.min.js" integrity="sha384-KJ3o2DKtIkvYIK3UENzmM7KCkRr/rE9/Qpg6aAZGJwFDMVNA/GpGFF93hXpG5KkN" crossorigin="anonymous"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/popper.js/1.12.9/umd/popper.min.js" integrity="sha384-ApNbgh9B+Y1QKtv3Rn7W3mgPxhU9K/ScQsAP7hUibX39j7fakFPskvXusvfa0b4Q" crossorigin="anonymous"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/4.0.0/js/bootstrap.min.js" integrity="sha384-JZR6Spejh4U02d8jOt6vLEHfe/JQGiRRSQQxSfFWpi1MquVdAyjUar5+76PVCmYl" crossorigin="anonymous"></script>
<script>
$(document).on('click', 'button[data-toggle="fast-collapse"]', function(e) {
  var target = $(this).data('target');
  if ($(target).filter(':visible').length === 0) {
    $(target).show();
  } else {
    $(target).hide();
  }
})
</script>
</body>
</html>
`

	WarningHeader = `
<div  style="background-color:pink" class="jumbotron">
  <h1>Warning: Analysis Error</h1>
  %s
</div>
`
)
