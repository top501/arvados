from cwltool.draft2tool import CommandLineTool
from .arvjob import ArvadosJob
from .arvcontainer import ArvadosContainer
from .pathmapper import ArvPathMapper

class ArvadosCommandTool(CommandLineTool):
    """Wrap cwltool CommandLineTool to override selected methods."""

    def __init__(self, arvrunner, toolpath_object, **kwargs):
        super(ArvadosCommandTool, self).__init__(toolpath_object, **kwargs)
        self.arvrunner = arvrunner
        self.work_api = kwargs["work_api"]

    def makeJobRunner(self):
        if self.work_api == "containers":
            return ArvadosContainer(self.arvrunner)
        elif self.work_api == "jobs":
            return ArvadosJob(self.arvrunner)

    def makePathMapper(self, reffiles, **kwargs):
        if self.work_api == "containers":
            return ArvPathMapper(self.arvrunner, reffiles, kwargs["basedir"],
                                 "/keep/%s",
                                 "/keep/%s/%s",
                                 **kwargs)
        elif self.work_api == "jobs":
            return ArvPathMapper(self.arvrunner, reffiles, kwargs["basedir"],
                                 "$(task.keep)/%s",
                                 "$(task.keep)/%s/%s",
                                 **kwargs)

    def job(self, joborder, output_callback, **kwargs):
        if self.work_api == "containers":
            kwargs["outdir"] = "/var/spool/cwl"
        elif self.work_api == "jobs":
            kwargs["outdir"] = "$(task.outdir)"
        return super(ArvadosCommandTool, self).job(joborder, output_callback, **kwargs)
